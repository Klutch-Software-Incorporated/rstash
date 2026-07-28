using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;

namespace Rstash.Database.Configurations;

internal sealed class OidcKeyConfiguration : IEntityTypeConfiguration<OidcKey>
{
    public void Configure(EntityTypeBuilder<OidcKey> builder)
    {
        builder.ToTable("oidc_keys");
        builder.HasKey(k => k.Purpose);

        builder.Property(k => k.Purpose).HasMaxLength(32);
        builder.Property(k => k.KeyMaterial).IsRequired();
    }
}
