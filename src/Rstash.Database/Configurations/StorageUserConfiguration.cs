using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;

namespace Rstash.Database.Configurations;

internal sealed class StorageUserConfiguration : IEntityTypeConfiguration<StorageUser>
{
    public void Configure(EntityTypeBuilder<StorageUser> builder)
    {
        builder.ToTable("storage_users");
        builder.HasKey(s => s.Id);

        builder.Property(s => s.Subject).HasMaxLength(255).IsRequired();
        builder.Property(s => s.UserName).HasMaxLength(255).IsRequired();
        builder.Property(s => s.NormalizedUserName).HasMaxLength(255).IsRequired();
        builder.Property(s => s.Plan).HasMaxLength(64).IsRequired();
        builder.Property(s => s.SourceIssuer).HasMaxLength(255);

        // The join key from the identity provider, and the name the remoteStorage
        // protocol addresses. Both must be unique instance-wide: a second row for the
        // same subject would fork one person's storage, and a duplicate username would
        // make /storage/{user}/… ambiguous. Uniqueness is on the normalized name so
        // the constraint is case-insensitive, matching how the lookup works.
        builder.HasIndex(s => s.Subject).IsUnique();
        builder.HasIndex(s => s.NormalizedUserName).IsUnique();
    }
}
