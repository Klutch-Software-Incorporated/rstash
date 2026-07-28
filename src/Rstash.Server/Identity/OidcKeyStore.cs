using System.Security.Cryptography;
using Microsoft.AspNetCore.DataProtection;
using Microsoft.EntityFrameworkCore;
using Microsoft.IdentityModel.Tokens;
using Rstash.Database;

namespace Rstash.Server.Identity;

/// <summary>
/// Loads the embedded provider's signing and encryption keys from the database,
/// generating them on first boot.
/// </summary>
/// <remarks>
/// OpenIddict's ephemeral helpers regenerate on every start, which breaks logins that
/// were mid-flight across a restart and — more seriously — makes a multi-instance
/// deployment fail intermittently, because each instance signs with a key the others
/// cannot validate. Persisting is what makes the provider survive both.
/// </remarks>
internal static class OidcKeyStore
{
    private const string SigningPurpose = "signing";
    private const string EncryptionPurpose = "encryption";

    /// <summary>Scopes the Data Protection payload so a protected value cannot be
    /// swapped between purposes.</summary>
    private const string ProtectorPurpose = "Rstash.Oidc.Keys.v1";

    private const int KeySizeBits = 2048;

    public static async Task<(SecurityKey Signing, SecurityKey Encryption)> LoadOrCreateAsync(
        IServiceProvider services)
    {
        await using var scope = services.CreateAsyncScope();
        var contextFactory = scope.ServiceProvider
            .GetRequiredService<IDbContextFactory<RstashDbContext>>();
        var protector = scope.ServiceProvider
            .GetRequiredService<IDataProtectionProvider>()
            .CreateProtector(ProtectorPurpose);

        return (
            await LoadOrCreateAsync(contextFactory, protector, SigningPurpose),
            await LoadOrCreateAsync(contextFactory, protector, EncryptionPurpose));
    }

    private static async Task<SecurityKey> LoadOrCreateAsync(
        IDbContextFactory<RstashDbContext> contextFactory,
        IDataProtector protector,
        string purpose)
    {
        await using var db = await contextFactory.CreateDbContextAsync();

        var existing = await db.OidcKeys.FirstOrDefaultAsync(k => k.Purpose == purpose);
        if (existing is not null)
        {
            return Import(protector.Unprotect(existing.KeyMaterial), purpose);
        }

        var rsa = RSA.Create(KeySizeBits);
        var material = protector.Protect(Convert.ToBase64String(rsa.ExportPkcs8PrivateKey()));

        db.OidcKeys.Add(new OidcKey
        {
            Purpose = purpose,
            KeyMaterial = material,
            CreatedAt = DateTimeOffset.UtcNow,
        });

        try
        {
            await db.SaveChangesAsync();
        }
        catch (DbUpdateException)
        {
            // Two instances starting together can race here. Purpose is the primary
            // key, so exactly one insert wins and the loser re-reads rather than
            // running on a key nobody else will honour.
            rsa.Dispose();
            await using var reread = await contextFactory.CreateDbContextAsync();
            var winner = await reread.OidcKeys.FirstAsync(k => k.Purpose == purpose);
            return Import(protector.Unprotect(winner.KeyMaterial), purpose);
        }

        return new RsaSecurityKey(rsa) { KeyId = KeyId(purpose) };
    }

    private static SecurityKey Import(string material, string purpose)
    {
        var rsa = RSA.Create();
        rsa.ImportPkcs8PrivateKey(Convert.FromBase64String(material), out _);
        return new RsaSecurityKey(rsa) { KeyId = KeyId(purpose) };
    }

    /// <summary>
    /// Stable per purpose, so the JWKS keeps advertising the same <c>kid</c> across
    /// restarts and relying parties do not have to refetch on every boot.
    /// </summary>
    private static string KeyId(string purpose) => $"rstash-{purpose}";
}
