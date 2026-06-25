using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;
using Rstash.Model;

namespace Rstash.Database.Configurations;

internal sealed class AuditEntryConfiguration : IEntityTypeConfiguration<AuditEntry>
{
    public void Configure(EntityTypeBuilder<AuditEntry> builder)
    {
        builder.ToTable("audit_log");
        builder.HasKey(a => a.Id);

        builder.Property(a => a.Action).HasMaxLength(255).IsRequired();
        builder.Property(a => a.TargetType).HasMaxLength(255).IsRequired();
        builder.Property(a => a.TargetId).HasMaxLength(255).IsRequired();
        builder.Property(a => a.Details).HasMaxLength(4096).HasDefaultValue(string.Empty);

        builder.HasIndex(a => a.CreatedAt);
    }
}
