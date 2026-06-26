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
}
