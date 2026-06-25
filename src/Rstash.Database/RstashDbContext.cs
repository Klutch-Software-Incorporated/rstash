using Microsoft.EntityFrameworkCore;
using Rstash.Model;

namespace Rstash.Database;

/// <summary>
/// The rstash EF Core context. Identity and OpenIddict stores are layered onto
/// this base in P3/P4; for now it owns the protocol domain tables.
/// </summary>
public class RstashDbContext(DbContextOptions<RstashDbContext> options) : DbContext(options)
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
