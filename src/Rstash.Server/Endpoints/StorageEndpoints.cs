using System.Buffers;
using System.Text.Json;
using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;
using Rstash.Services;
using Rstash.Services.Storage;

namespace Rstash.Server.Endpoints;

/// <summary>
/// The remoteStorage storage API: GET/PUT/DELETE/HEAD on /storage/{user}/{path}.
/// Bearer-token auth + scope checks (public documents are readable anonymously),
/// conditional requests, and JSON-LD folder listings.
/// </summary>
internal static class StorageEndpoints
{
    public static void MapStorageEndpoints(this IEndpointRouteBuilder endpoints)
    {
        endpoints
            .MapMethods("/storage/{user}/{**path}", ["GET", "HEAD", "PUT", "DELETE"], HandleAsync)
            .RequireCors("rstash-storage");
    }

    private static async Task HandleAsync(
        HttpContext ctx,
        string user,
        string? path,
        RemoteStorageService storage,
        EgressTracker egress,
        TokenStore tokens,
        IDbContextFactory<RstashDbContext> contextFactory,
        SettingsService settings,
        AuditService audit)
    {
        var storagePath = "/" + (path ?? string.Empty);

        if (!StoragePath.TryValidate(storagePath, out var pathError))
        {
            await TextAsync(ctx, StatusCodes.Status400BadRequest, $"invalid path: {pathError}");
            return;
        }

        var isFolder = storagePath.EndsWith('/');
        var isPublic = storagePath.StartsWith("/public/", StringComparison.Ordinal);
        var method = ctx.Request.Method;
        var isReadOnly = method is "GET" or "HEAD";
        var isWrite = method is "PUT" or "DELETE";

        // Public documents (not folders) are readable without auth.
        var needsAuth = !(isPublic && isReadOnly && !isFolder);

        long tokenUserId = 0;
        if (needsAuth)
        {
            var bearer = ExtractBearer(ctx.Request);
            if (string.IsNullOrEmpty(bearer))
            {
                ctx.Response.Headers.WWWAuthenticate = "Bearer";
                await TextAsync(ctx, StatusCodes.Status401Unauthorized, "unauthorized");
                return;
            }

            var token = await tokens.ValidateAsync(bearer, ctx.RequestAborted);
            if (token is null)
            {
                ctx.Response.Headers.WWWAuthenticate = "Bearer error=\"invalid_token\"";
                await TextAsync(ctx, StatusCodes.Status401Unauthorized, "invalid token");
                return;
            }

            if (!Scope.TryParse(token.Scopes, out var scopes) || !Scope.Grants(scopes, storagePath, isWrite))
            {
                await TextAsync(ctx, StatusCodes.Status403Forbidden, "insufficient scope");
                return;
            }

            tokenUserId = token.UserId;
        }

        // One query per request resolves the owner and everything enforced below: the
        // admin kill switch, the approval gate, and the egress cap. It stays on the
        // request path rather than being baked into the token so that disabling an
        // account takes effect immediately.
        var normalized = user.ToUpperInvariant();
        long ownerId;
        long maxEgress;
        bool barred;
        await using (var db = await contextFactory.CreateDbContextAsync(ctx.RequestAborted))
        {
            var owner = await db.Users
                .Where(u => u.NormalizedUserName == normalized)
                .Select(u => new { u.Id, u.EgressQuota, Barred = u.Disabled || !u.Approved })
                .FirstOrDefaultAsync(ctx.RequestAborted);

            ownerId = owner?.Id ?? 0;
            maxEgress = owner?.EgressQuota ?? 0;
            barred = owner?.Barred ?? true;
        }

        if (ownerId == 0)
        {
            await TextAsync(ctx, StatusCodes.Status404NotFound, "user not found");
            return;
        }

        if (needsAuth && barred)
        {
            await TextAsync(ctx, StatusCodes.Status403Forbidden, "account disabled");
            return;
        }

        if (needsAuth && tokenUserId != ownerId)
        {
            await TextAsync(ctx, StatusCodes.Status403Forbidden, "forbidden");
            return;
        }

        // Operator policy: writes/deletes under /public/ can be disabled server-wide.
        // Public documents stay readable; owners manage existing ones via the web UI.
        if (isPublic && isWrite && !settings.Current.AllowPublicWrites)
        {
            await TextAsync(ctx, StatusCodes.Status403Forbidden, "public writes are disabled");
            return;
        }

        var conditions = ParseConditions(ctx.Request);

        try
        {
            switch (method)
            {
                case "GET" when isFolder:
                    await GetFolderAsync(ctx, storage, ownerId, storagePath, isPublic, conditions, writeBody: true);
                    break;
                case "HEAD" when isFolder:
                    await GetFolderAsync(ctx, storage, ownerId, storagePath, isPublic, conditions, writeBody: false);
                    break;
                case "GET":
                    await GetDocumentAsync(
                        ctx, storage, egress, ownerId, maxEgress, storagePath, isPublic, conditions);
                    break;
                case "HEAD":
                    await HeadDocumentAsync(ctx, storage, ownerId, storagePath, isPublic, conditions);
                    break;
                case "PUT" when isFolder:
                    await TextAsync(ctx, StatusCodes.Status400BadRequest, "cannot PUT a folder");
                    break;
                case "PUT":
                    if (await PutDocumentAsync(ctx, storage, ownerId, storagePath, conditions, settings.Current.MaxUploadSize))
                    {
                        await audit.RecordAsync(ownerId, "storage.put", "storage", storagePath);
                    }

                    break;
                case "DELETE":
                    await DeleteDocumentAsync(ctx, storage, ownerId, storagePath, conditions);
                    await audit.RecordAsync(ownerId, "storage.delete", "storage", storagePath);
                    break;
                default:
                    await TextAsync(ctx, StatusCodes.Status405MethodNotAllowed, "method not allowed");
                    break;
            }
        }
        catch (StorageException ex)
        {
            await WriteServiceErrorAsync(ctx, ex);
        }
    }

    private static async Task GetDocumentAsync(
        HttpContext ctx, RemoteStorageService storage, EgressTracker egress, long userId, long userEgressQuota,
        string path, bool isPublic, StorageConditions cond)
    {
        var result = await storage.GetDocumentAsync(userId, path, cond, ctx.RequestAborted);
        await using var content = result.Content;

        // Egress metering: refuse the whole download if it would blow the per-user or global
        // monthly cap. Charged to the storage owner, so anonymous /public/ reads count too.
        if (!await egress.CanServeAsync(userId, result.ContentLength, userEgressQuota, ctx.RequestAborted))
        {
            await WriteEgressExceededAsync(ctx);
            return;
        }

        egress.Record(userId, result.ContentLength);

        ctx.Response.StatusCode = StatusCodes.Status200OK;
        ctx.Response.Headers.ETag = ETag.Quote(result.ETag);
        ctx.Response.ContentType = result.ContentType.Length > 0 ? result.ContentType : "application/octet-stream";
        ctx.Response.ContentLength = result.ContentLength;
        ctx.Response.Headers.CacheControl = CacheControl(isPublic);

        await content.CopyToAsync(ctx.Response.Body, ctx.RequestAborted);
    }

    // See EgressLimit for why this is a 429 with a Retry-After at the period boundary.
    private static Task WriteEgressExceededAsync(HttpContext ctx)
    {
        EgressLimit.SetRetryAfter(ctx.Response);
        return TextAsync(ctx, StatusCodes.Status429TooManyRequests, "egress exceeded");
    }

    private static async Task HeadDocumentAsync(
        HttpContext ctx, RemoteStorageService storage, long userId, string path, bool isPublic, StorageConditions cond)
    {
        var result = await storage.HeadDocumentAsync(userId, path, cond, ctx.RequestAborted);

        ctx.Response.StatusCode = StatusCodes.Status200OK;
        ctx.Response.Headers.ETag = ETag.Quote(result.ETag);
        ctx.Response.ContentType = result.ContentType.Length > 0 ? result.ContentType : "application/octet-stream";
        ctx.Response.ContentLength = result.ContentLength;
        ctx.Response.Headers.CacheControl = CacheControl(isPublic);
    }

    private static async Task GetFolderAsync(
        HttpContext ctx, RemoteStorageService storage, long userId, string path, bool isPublic,
        StorageConditions cond, bool writeBody)
    {
        var (description, etag) = await storage.GetFolderAsync(userId, path, cond, ctx.RequestAborted);
        var body = JsonSerializer.SerializeToUtf8Bytes(description);

        ctx.Response.StatusCode = StatusCodes.Status200OK;
        ctx.Response.Headers.ETag = ETag.Quote(etag);
        ctx.Response.ContentType = "application/ld+json";
        ctx.Response.ContentLength = body.Length;
        ctx.Response.Headers.CacheControl = CacheControl(isPublic);

        if (writeBody)
        {
            await ctx.Response.Body.WriteAsync(body, ctx.RequestAborted);
        }
    }

    private static async Task<bool> PutDocumentAsync(
        HttpContext ctx, RemoteStorageService storage, long userId, string path,
        StorageConditions cond, long maxUploadSize)
    {
        var contentType = ctx.Request.ContentType;
        if (string.IsNullOrEmpty(contentType))
        {
            contentType = "application/octet-stream";
        }

        if (ctx.Request.ContentLength is { } declared && declared > maxUploadSize)
        {
            await TextAsync(ctx, StatusCodes.Status413PayloadTooLarge, "payload too large");
            return false;
        }

        var data = await ReadCappedAsync(ctx.Request.Body, maxUploadSize, ctx.RequestAborted);
        if (data is null)
        {
            await TextAsync(ctx, StatusCodes.Status413PayloadTooLarge, "payload too large");
            return false;
        }

        using var buffered = new MemoryStream(data, writable: false);
        var result = await storage.PutDocumentAsync(userId, path, buffered, contentType, cond, ctx.RequestAborted);

        ctx.Response.Headers.ETag = ETag.Quote(result.ETag);
        ctx.Response.StatusCode = result.IsNew ? StatusCodes.Status201Created : StatusCodes.Status200OK;
        return true;
    }

    private static async Task DeleteDocumentAsync(
        HttpContext ctx, RemoteStorageService storage, long userId, string path, StorageConditions cond)
    {
        var result = await storage.DeleteDocumentAsync(userId, path, cond, ctx.RequestAborted);

        ctx.Response.Headers.ETag = ETag.Quote(result.ETag);
        ctx.Response.StatusCode = StatusCodes.Status200OK;
    }

    private static async Task WriteServiceErrorAsync(HttpContext ctx, StorageException ex)
    {
        switch (ex.Error)
        {
            case StorageError.NotModified:
                if (ex.ETag is { } etag)
                {
                    ctx.Response.Headers.ETag = ETag.Quote(etag);
                }

                ctx.Response.StatusCode = StatusCodes.Status304NotModified;
                break;
            case StorageError.NotFound:
                await TextAsync(ctx, StatusCodes.Status404NotFound, "not found");
                break;
            case StorageError.PreconditionFailed:
                await TextAsync(ctx, StatusCodes.Status412PreconditionFailed, "precondition failed");
                break;
            case StorageError.Conflict:
                await TextAsync(ctx, StatusCodes.Status409Conflict, "conflict");
                break;
            case StorageError.PayloadTooLarge:
                await TextAsync(ctx, StatusCodes.Status413PayloadTooLarge, "payload too large");
                break;
            case StorageError.ContentRejected:
                await TextAsync(ctx, StatusCodes.Status415UnsupportedMediaType, ex.Message);
                break;
            case StorageError.QuotaExceeded:
                await TextAsync(ctx, StatusCodes.Status507InsufficientStorage, "quota exceeded");
                break;
            default:
                await TextAsync(ctx, StatusCodes.Status500InternalServerError, "internal error");
                break;
        }
    }

    private static string CacheControl(bool isPublic) => isPublic ? "no-cache, public" : "no-cache";

    private static string? ExtractBearer(HttpRequest request)
    {
        var header = request.Headers.Authorization.ToString();
        return header.StartsWith("Bearer ", StringComparison.Ordinal) ? header["Bearer ".Length..] : null;
    }

    private static StorageConditions ParseConditions(HttpRequest request)
    {
        string? ifMatch = null;
        var ifMatchHeader = request.Headers.IfMatch.ToString();
        if (!string.IsNullOrEmpty(ifMatchHeader))
        {
            ifMatch = ETag.Unquote(ifMatchHeader);
        }

        var ifNoneMatch = new List<string>();
        var ifNoneMatchHeader = request.Headers.IfNoneMatch.ToString();
        if (ifNoneMatchHeader == "*")
        {
            ifNoneMatch.Add("*");
        }
        else if (!string.IsNullOrEmpty(ifNoneMatchHeader))
        {
            foreach (var part in ifNoneMatchHeader.Split(','))
            {
                var trimmed = part.Trim();
                if (trimmed.Length > 0)
                {
                    ifNoneMatch.Add(ETag.Unquote(trimmed));
                }
            }
        }

        return new StorageConditions { IfMatch = ifMatch, IfNoneMatch = ifNoneMatch };
    }

    private static async Task<byte[]?> ReadCappedAsync(Stream body, long max, CancellationToken cancellationToken)
    {
        using var buffer = new MemoryStream();
        var rented = ArrayPool<byte>.Shared.Rent(81920);
        try
        {
            int read;
            while ((read = await body.ReadAsync(rented, cancellationToken)) > 0)
            {
                if (buffer.Length + read > max)
                {
                    return null;
                }

                buffer.Write(rented, 0, read);
            }

            return buffer.ToArray();
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(rented);
        }
    }

    private static async Task TextAsync(HttpContext ctx, int statusCode, string message)
    {
        ctx.Response.StatusCode = statusCode;
        ctx.Response.ContentType = "text/plain";
        await ctx.Response.WriteAsync(message);
    }
}
