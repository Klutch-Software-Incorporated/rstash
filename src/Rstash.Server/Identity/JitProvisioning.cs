using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Server.Identity;

/// <summary>
/// Creates the storage-side user record from the claims on a completed login.
/// </summary>
/// <remarks>
/// Runs in embedded mode too, even though the account already exists locally, so that
/// exactly one code path turns claims into a <see cref="StorageUser"/>. When the
/// provider moves out, this is unchanged — it never reads the Identity tables.
/// </remarks>
internal static class JitProvisioning
{
    public static async Task OnTokenValidatedAsync(TokenValidatedContext context)
    {
        var principal = context.Principal
            ?? throw new InvalidOperationException("token validated without a principal");

        var subject = principal.FindFirst("sub")?.Value;
        if (string.IsNullOrWhiteSpace(subject))
        {
            context.Fail("the identity provider returned no subject claim");
            return;
        }

        var userName = principal.FindFirst("preferred_username")?.Value;
        if (string.IsNullOrWhiteSpace(userName))
        {
            context.Fail("the identity provider returned no preferred_username claim");
            return;
        }

        var contextFactory = context.HttpContext.RequestServices
            .GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();

        var normalized = userName.ToUpperInvariant();
        var existing = await db.StorageUsers.FirstOrDefaultAsync(s => s.Subject == subject);

        if (existing is not null)
        {
            // The username is immutable once storage exists: it appears in every
            // storage URL and in WebFinger, so renaming is a data migration rather
            // than a claim update. Keep ours and let the divergence be visible.
            if (!string.Equals(existing.NormalizedUserName, normalized, StringComparison.Ordinal))
            {
                context.HttpContext.RequestServices
                    .GetRequiredService<ILoggerFactory>()
                    .CreateLogger(typeof(JitProvisioning))
                    .LogWarning(
                        "Provider reports username {Claimed} for subject {Subject}, but "
                        + "storage holds {Current}; keeping {Current}.",
                        userName,
                        subject,
                        existing.UserName);
            }

            return;
        }

        // Decision 2's collision rule. Two subjects cannot share a username, because
        // /storage/{user}/… would be ambiguous and WebFinger would resolve one person's
        // address to another's data. Failing the login is the only safe answer — the
        // alternative is silently handing someone else's storage to whoever logged in.
        if (await db.StorageUsers.AnyAsync(s => s.NormalizedUserName == normalized))
        {
            context.Fail(
                $"username '{userName}' already belongs to a different account on this server");
            return;
        }

        db.StorageUsers.Add(new StorageUser
        {
            Subject = subject,
            UserName = userName,
            NormalizedUserName = normalized,
            Plan = "",
            CreatedAt = DateTimeOffset.UtcNow,

            // Entitlements stay at their zero values until something authoritative
            // supplies them: locally that is the admin UI, externally the control
            // plane. Zero means unlimited, matching how the settings defaults behave.
            SourceIssuer = context.Options.Authority,
        });

        try
        {
            await db.SaveChangesAsync();
        }
        catch (DbUpdateException)
        {
            // Concurrent first logins for the same subject: the unique index on Subject
            // means one wins, and the loser is fine — the row it wanted now exists.
            if (!await db.StorageUsers.AnyAsync(s => s.Subject == subject))
            {
                throw;
            }
        }
    }
}
