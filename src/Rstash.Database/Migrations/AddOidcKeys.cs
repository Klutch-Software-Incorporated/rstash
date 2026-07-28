using FluentMigrator;

namespace Rstash.Database.Migrations;

/// <summary>
/// Persistent key material for the embedded OpenID Connect provider.
///
/// OpenIddict's development helpers generate keys in memory, which fails in two ways
/// that matter: a restart invalidates any login that was mid-flight across it, and —
/// worse — two instances behind a load balancer each sign with a key the other
/// cannot validate, so logins fail intermittently and only under scale-out.
///
/// One row per purpose (<c>signing</c>, <c>encryption</c>), holding RSA parameters
/// rather than a certificate: OpenIddict accepts a bare <c>SecurityKey</c>, and that
/// avoids X.509 lifetime and store handling for something never presented to a peer.
/// </summary>
[Migration(202607280004, "Persistent OpenID Connect signing and encryption keys")]
public sealed class AddOidcKeys : Migration
{
    public override void Up()
    {
        Create.Table("oidc_keys")
            // "signing" or "encryption" — the purpose is the key, so a row cannot be
            // duplicated and a concurrent first boot cannot create two of either.
            .WithColumn("Purpose").AsString(32).NotNullable().PrimaryKey()
            .WithColumn("KeyMaterial").AsString(int.MaxValue).NotNullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable();
    }

    public override void Down()
    {
        Delete.Table("oidc_keys");
    }
}
