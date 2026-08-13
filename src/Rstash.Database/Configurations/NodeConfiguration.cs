using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;
using Rstash.Model;

namespace Rstash.Database.Configurations;

internal sealed class NodeConfiguration : IEntityTypeConfiguration<Node>
{
    public void Configure(EntityTypeBuilder<Node> builder)
    {
        builder.ToTable("nodes");
        builder.HasKey(n => n.Id);

        builder.Property(n => n.Path).HasMaxLength(512).IsRequired();
        builder.Property(n => n.ContentType).HasMaxLength(255).HasDefaultValue(string.Empty);
        builder.Property(n => n.ETag).HasMaxLength(255).IsRequired();

        // A document path is unique per user. Folder listings depend on prefix
        // matching staying case-sensitive, which on SQLite comes from a
        // connection interceptor setting PRAGMA case_sensitive_like rather than
        // from a collation on this index.
        builder.HasIndex(n => new { n.UserId, n.Path }).IsUnique();
    }
}
