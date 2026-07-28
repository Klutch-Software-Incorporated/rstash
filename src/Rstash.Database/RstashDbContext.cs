using Microsoft.AspNetCore.DataProtection.EntityFrameworkCore;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Identity.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore;
using Rstash.Model;

namespace Rstash.Database;

/// <summary>
/// The rstash EF Core context, built on Identity (AspNet* tables). OpenIddict
/// stores are layered on in P4; it also owns the protocol domain tables.
/// <para>
/// Cross-provider invariant: all <see cref="DateTimeOffset"/> values persisted here
/// must be UTC (offset 0). Npgsql maps <c>DateTimeOffset</c> to <c>timestamptz</c>
/// only when the offset is zero and throws otherwise; current writers use
/// <see cref="DateTimeOffset.UtcNow"/>, so any new one must too (or call
/// <c>.ToUniversalTime()</c>).
/// </para>
/// </summary>
public class RstashDbContext(DbContextOptions<RstashDbContext> options)
    : IdentityDbContext<ApplicationUser, IdentityRole<long>, long>(options), IDataProtectionKeyContext
{
    /// <summary>
    /// The Data Protection key ring. These keys encrypt the auth cookie and antiforgery
    /// tokens, so they must outlive the process — without persistence every restart
    /// invalidates every session. Written only by the Data Protection stack.
    /// </summary>
    public DbSet<DataProtectionKey> DataProtectionKeys => Set<DataProtectionKey>();

    public DbSet<Node> Nodes => Set<Node>();

    public DbSet<AuditEntry> AuditLog => Set<AuditEntry>();

    public DbSet<Setting> Settings => Set<Setting>();

    public DbSet<OAuthToken> OAuthTokens => Set<OAuthToken>();

    public DbSet<AuthorizationCode> AuthorizationCodes => Set<AuthorizationCode>();

    public DbSet<EgressUsage> EgressUsage => Set<EgressUsage>();

    protected override void OnModelCreating(ModelBuilder builder)
    {
        base.OnModelCreating(builder);
        builder.ApplyConfigurationsFromAssembly(typeof(RstashDbContext).Assembly);

        // The embedded OpenID Connect provider's four tables (applications,
        // authorizations, scopes, tokens). This registers only the EF model; the DDL
        // is hand-written in a FluentMigrator migration like everything else, and
        // OpenIddictSchemaTests guards the two against drifting.
        builder.UseOpenIddict();
    }
}
