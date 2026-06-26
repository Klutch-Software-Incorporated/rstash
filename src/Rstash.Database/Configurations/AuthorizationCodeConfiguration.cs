using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Metadata.Builders;

namespace Rstash.Database.Configurations;

internal sealed class AuthorizationCodeConfiguration : IEntityTypeConfiguration<AuthorizationCode>
{
    public void Configure(EntityTypeBuilder<AuthorizationCode> builder)
    {
        builder.ToTable("authorization_codes");
        builder.HasKey(c => c.Code);

        builder.Property(c => c.Code).HasMaxLength(255);
        builder.Property(c => c.ClientId).HasMaxLength(255);
        builder.Property(c => c.RedirectUri).HasMaxLength(2048);
        builder.Property(c => c.Scopes).HasMaxLength(1024);
        builder.Property(c => c.CodeChallenge).HasMaxLength(255);
        builder.Property(c => c.CodeChallengeMethod).HasMaxLength(32);
    }
}
