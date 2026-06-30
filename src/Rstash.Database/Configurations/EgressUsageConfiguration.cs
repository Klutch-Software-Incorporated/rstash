using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;

namespace Rstash.Database.Configurations;

internal sealed class EgressUsageConfiguration : IEntityTypeConfiguration<EgressUsage>
{
    public void Configure(EntityTypeBuilder<EgressUsage> builder)
    {
        builder.ToTable("egress_usage");
        builder.HasKey(e => e.Id);

        builder.Property(e => e.Period).HasMaxLength(7).IsRequired();

        // One counter per user per period; the unique index also serves the
        // per-user lookup on the read path.
        builder.HasIndex(e => new { e.UserId, e.Period }).IsUnique();
    }
}
