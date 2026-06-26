using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Identity;
using Rstash.Database;
using Rstash.Services.Storage;

namespace Rstash.Server.Endpoints;

/// <summary>
/// Cookie-authenticated file operations for the in-app browser (distinct from
/// the bearer-token storage API). Acts on the signed-in user's own storage.
/// The browser is read-only apart from deletes — content arrives via the
/// remoteStorage API, not by uploading through the UI.
/// </summary>
internal static class FileBrowserEndpoints
{
    public static void MapFileBrowserEndpoints(this IEndpointRouteBuilder endpoints)
    {
        var group = endpoints.MapGroup("/files").RequireAuthorization();
        group.MapGet("/download/{**path}", DownloadAsync);
        group.MapPost("/delete", DeleteAsync);
    }

    private static async Task<IResult> DownloadAsync(
        HttpContext ctx, string path, RemoteStorageService storage, UserManager<ApplicationUser> users)
    {
        var user = await users.GetUserAsync(ctx.User);
        if (user is null)
        {
            return Results.Unauthorized();
        }

        try
        {
            var result = await storage.GetDocumentAsync(user.Id, "/" + path, new StorageConditions(), ctx.RequestAborted);
            var name = path.Split('/').LastOrDefault() ?? "file";
            var contentType = result.ContentType.Length > 0 ? result.ContentType : "application/octet-stream";
            return Results.File(result.Content, contentType, name);
        }
        catch (StorageException ex) when (ex.Error == StorageError.NotFound)
        {
            return Results.NotFound();
        }
    }

    private static async Task<IResult> DeleteAsync(
        HttpContext ctx, IAntiforgery antiforgery, RemoteStorageService storage, UserManager<ApplicationUser> users)
    {
        if (!await ValidAntiforgery(ctx, antiforgery))
        {
            return Results.BadRequest();
        }

        var user = await users.GetUserAsync(ctx.User);
        if (user is null)
        {
            return Results.Unauthorized();
        }

        var form = await ctx.Request.ReadFormAsync();
        var folder = form["folder"].ToString();

        // One or more selected paths; folders (trailing '/') delete their whole subtree.
        foreach (var path in form["paths"])
        {
            if (string.IsNullOrEmpty(path))
            {
                continue;
            }

            try
            {
                if (path.EndsWith('/'))
                {
                    await storage.DeleteFolderAsync(user.Id, path, ctx.RequestAborted);
                }
                else
                {
                    await storage.DeleteDocumentAsync(user.Id, path, new StorageConditions(), ctx.RequestAborted);
                }
            }
            catch (StorageException)
            {
                // Already gone (or removed as part of a selected parent folder) — fine.
            }
        }

        return Results.Redirect("/files" + folder);
    }

    private static async Task<bool> ValidAntiforgery(HttpContext ctx, IAntiforgery antiforgery)
    {
        try
        {
            await antiforgery.ValidateRequestAsync(ctx);
            return true;
        }
        catch (AntiforgeryValidationException)
        {
            return false;
        }
    }
}
