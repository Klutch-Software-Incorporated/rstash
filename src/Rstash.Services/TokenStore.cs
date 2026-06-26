using System.Security.Cryptography;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Services;

/// <summary>
/// Issues and validates remoteStorage access tokens. Tokens are opaque 32-byte
/// random values (hex). Validation rejects unknown or expired tokens.
/// </summary>
public sealed class TokenStore(IDbContextFactory<RstashDbContext> contextFactory)
{
    public async Task<OAuthToken?> ValidateAsync(string token, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var found = await db.OAuthTokens.AsNoTracking()
            .FirstOrDefaultAsync(t => t.Token == token, cancellationToken);

        if (found is null)
        {
            return null;
        }

        if (found.ExpiresAt is { } expiresAt && expiresAt <= DateTimeOffset.UtcNow)
        {
            return null;
        }

        return found;
    }

    public async Task<OAuthToken> CreateAsync(
        long userId, string clientId, string scopes, TimeSpan? lifetime,
        CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var token = new OAuthToken
        {
            Token = Convert.ToHexStringLower(RandomNumberGenerator.GetBytes(32)),
            UserId = userId,
            ClientId = clientId,
            Scopes = scopes,
            CreatedAt = now,
            ExpiresAt = lifetime is { } span ? now.Add(span) : null,
        };

        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        db.OAuthTokens.Add(token);
        await db.SaveChangesAsync(cancellationToken);
        return token;
    }

    public async Task RevokeAsync(string token, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        await db.OAuthTokens.Where(t => t.Token == token).ExecuteDeleteAsync(cancellationToken);
    }

    // ── Authorization codes (one-time, PKCE) ──

    public async Task<AuthorizationCode> CreateCodeAsync(
        long userId, string clientId, string redirectUri, string scopes,
        string codeChallenge, string codeChallengeMethod, CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var code = new AuthorizationCode
        {
            Code = Convert.ToHexStringLower(RandomNumberGenerator.GetBytes(32)),
            UserId = userId,
            ClientId = clientId,
            RedirectUri = redirectUri,
            Scopes = scopes,
            CodeChallenge = codeChallenge,
            CodeChallengeMethod = codeChallengeMethod,
            CreatedAt = now,
            ExpiresAt = now.AddMinutes(10),
            Used = false,
        };

        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        db.AuthorizationCodes.Add(code);
        await db.SaveChangesAsync(cancellationToken);
        return code;
    }

    /// <summary>Returns the code if it exists, is unused, and is not expired.</summary>
    public async Task<AuthorizationCode?> GetValidCodeAsync(string code, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var found = await db.AuthorizationCodes.AsNoTracking()
            .FirstOrDefaultAsync(c => c.Code == code, cancellationToken);

        if (found is null || found.Used || found.ExpiresAt <= DateTimeOffset.UtcNow)
        {
            return null;
        }

        return found;
    }

    public async Task UseCodeAsync(string code, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        await db.AuthorizationCodes
            .Where(c => c.Code == code)
            .ExecuteUpdateAsync(setters => setters.SetProperty(c => c.Used, true), cancellationToken);
    }
}
