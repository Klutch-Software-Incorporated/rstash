using Microsoft.EntityFrameworkCore;

namespace Rstash.Storage;

/// <summary>
/// A minimal context for the database blob backend — a single <c>blobs</c>
/// table living in the (possibly separate) blob database. Created via
/// EnsureCreated rather than migrations, since it is a standalone aux schema.
/// </summary>
public sealed class BlobDbContext(DbContextOptions<BlobDbContext> options) : DbContext(options)
{
    public DbSet<Blob> Blobs => Set<Blob>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        base.OnModelCreating(modelBuilder);

        var blob = modelBuilder.Entity<Blob>();
        blob.ToTable("blobs");
        blob.HasKey(b => new { b.UserId, b.Path });
        blob.Property(b => b.Path).HasMaxLength(512);
        blob.Property(b => b.Data).IsRequired();
    }
}
