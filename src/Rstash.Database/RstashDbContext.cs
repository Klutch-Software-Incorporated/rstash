using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Identity.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore;
using Rstash.Model;

namespace Rstash.Database;

/// <summary>
/// The rstash EF Core context, built on Identity (AspNet* tables). OpenIddict
/// stores are layered on in P4; it also owns the protocol domain tables.
/// </summary>
public class RstashDbContext(DbContextOptions<RstashDbContext> options)
    : IdentityDbContext<ApplicationUser, IdentityRole<long>, long>(options)
{
    public DbSet<Node> Nodes => Set<Node>();

    public DbSet<AuditEntry> AuditLog => Set<AuditEntry>();

    public DbSet<Setting> Settings => Set<Setting>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        base.OnModelCreating(modelBuilder);
        modelBuilder.ApplyConfigurationsFromAssembly(typeof(RstashDbContext).Assembly);
    }
}
