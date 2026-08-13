using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;

namespace Rstash.Database.Configurations;

internal sealed class OAuthTokenConfiguration : IEntityTypeConfiguration<OAuthToken>
{
    public void Configure(EntityTypeBuilder<OAuthToken> builder)
    {
        builder.ToTable("oauth_tokens");
        builder.HasKey(t => t.Token);

        builder.Property(t => t.Token).HasMaxLength(255);
        builder.Property(t => t.ClientId).HasMaxLength(255);
        builder.Property(t => t.Scopes).HasMaxLength(1024);
        builder.Property(t => t.RefreshToken).HasMaxLength(255);

        builder.HasIndex(t => t.UserId);
        builder.HasIndex(t => t.RefreshToken).HasDatabaseName("IX_oauth_tokens_RefreshToken");
    }
}
