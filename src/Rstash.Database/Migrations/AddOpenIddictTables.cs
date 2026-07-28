using System.Data;
using FluentMigrator;

namespace Rstash.Database.Migrations;

/// <summary>
/// The embedded OpenID Connect provider's four tables. rstash always authenticates
/// humans as an OIDC relying party; self-hosters get the provider bundled in the
/// same binary, and these are its stores.
///
/// Not to be confused with <c>oauth_tokens</c> / <c>authorization_codes</c>, which
/// belong to the remoteStorage app-authorization server. That one authorizes *apps*
/// against a spec-defined OAuth profile and stays exactly as it is; these tables
/// authorize *humans*. Both are token stores, and merging them would be a mistake.
///
/// Column names, nullability, and lengths mirror OpenIddict's EF Core model so EF
/// can trust this schema (guarded by <c>OpenIddictSchemaTests</c>). One deliberate
/// divergence: the model leaves the key and foreign-key columns unbounded, but an
/// unbounded string cannot be a primary key on SQL Server and needs an explicit
/// length on MySQL. They are bounded to 450 here — OpenIddict's own choice for SQL
/// Server, and far beyond the 36 characters a GUID actually needs. Being stricter
/// than the model is safe; EF only reads and writes through it.
/// </summary>
[Migration(202607280002, "OpenIddict stores for the embedded OpenID Connect provider")]
public sealed class AddOpenIddictTables : Migration
{
    /// <summary>Bounds the key columns; see the divergence note above.</summary>
    private const int KeyLength = 450;

    public override void Up()
    {
        Create.Table("OpenIddictApplications")
            .WithColumn("Id").AsString(KeyLength).NotNullable().PrimaryKey()
            .WithColumn("ApplicationType").AsString(50).Nullable()
            .WithColumn("ClientId").AsString(100).Nullable()
            .WithColumn("ClientSecret").AsString(int.MaxValue).Nullable()
            .WithColumn("ClientType").AsString(50).Nullable()
            .WithColumn("ConcurrencyToken").AsString(50).Nullable()
            .WithColumn("ConsentType").AsString(50).Nullable()
            .WithColumn("DisplayName").AsString(int.MaxValue).Nullable()
            .WithColumn("DisplayNames").AsString(int.MaxValue).Nullable()
            .WithColumn("JsonWebKeySet").AsString(int.MaxValue).Nullable()
            .WithColumn("Permissions").AsString(int.MaxValue).Nullable()
            .WithColumn("PostLogoutRedirectUris").AsString(int.MaxValue).Nullable()
            .WithColumn("Properties").AsString(int.MaxValue).Nullable()
            .WithColumn("RedirectUris").AsString(int.MaxValue).Nullable()
            .WithColumn("Requirements").AsString(int.MaxValue).Nullable()
            .WithColumn("Settings").AsString(int.MaxValue).Nullable();

        Create.Table("OpenIddictScopes")
            .WithColumn("Id").AsString(KeyLength).NotNullable().PrimaryKey()
            .WithColumn("ConcurrencyToken").AsString(50).Nullable()
            .WithColumn("Description").AsString(int.MaxValue).Nullable()
            .WithColumn("Descriptions").AsString(int.MaxValue).Nullable()
            .WithColumn("DisplayName").AsString(int.MaxValue).Nullable()
            .WithColumn("DisplayNames").AsString(int.MaxValue).Nullable()
            .WithColumn("Name").AsString(200).Nullable()
            .WithColumn("Properties").AsString(int.MaxValue).Nullable()
            .WithColumn("Resources").AsString(int.MaxValue).Nullable();

        Create.Table("OpenIddictAuthorizations")
            .WithColumn("Id").AsString(KeyLength).NotNullable().PrimaryKey()
            .WithColumn("ApplicationId").AsString(KeyLength).Nullable()
            .WithColumn("ConcurrencyToken").AsString(50).Nullable()
            // DateTime, not DateTimeOffset: OpenIddict's models use plain DateTime for
            // every timestamp, unlike rstash's own tables. Using AsDateTimeOffset here
            // silently mismatches the EF model on any provider that distinguishes them.
            .WithColumn("CreationDate").AsDateTime().Nullable()
            .WithColumn("Properties").AsString(int.MaxValue).Nullable()
            .WithColumn("Scopes").AsString(int.MaxValue).Nullable()
            .WithColumn("Status").AsString(50).Nullable()
            .WithColumn("Subject").AsString(400).Nullable()
            .WithColumn("Type").AsString(50).Nullable();

        Create.Table("OpenIddictTokens")
            .WithColumn("Id").AsString(KeyLength).NotNullable().PrimaryKey()
            .WithColumn("ApplicationId").AsString(KeyLength).Nullable()
            .WithColumn("AuthorizationId").AsString(KeyLength).Nullable()
            .WithColumn("ConcurrencyToken").AsString(50).Nullable()
            // DateTime, not DateTimeOffset — see the note on OpenIddictAuthorizations.
            .WithColumn("CreationDate").AsDateTime().Nullable()
            .WithColumn("ExpirationDate").AsDateTime().Nullable()
            .WithColumn("Payload").AsString(int.MaxValue).Nullable()
            .WithColumn("Properties").AsString(int.MaxValue).Nullable()
            .WithColumn("RedemptionDate").AsDateTime().Nullable()
            .WithColumn("ReferenceId").AsString(100).Nullable()
            .WithColumn("Status").AsString(50).Nullable()
            .WithColumn("Subject").AsString(400).Nullable()
            .WithColumn("Type").AsString(150).Nullable();

        CreateForeignKeys();
        CreateIndexes();
    }

    public override void Down()
    {
        // Dependents first (FK order), then principals.
        Delete.Table("OpenIddictTokens");
        Delete.Table("OpenIddictAuthorizations");
        Delete.Table("OpenIddictScopes");
        Delete.Table("OpenIddictApplications");
    }

    private void CreateForeignKeys()
    {
        // No cascade: OpenIddict prunes expired tokens and orphaned authorizations
        // itself, and a cascading delete would let removing an application silently
        // discard its audit-relevant token history.
        Create.ForeignKey("FK_OpenIddictAuthorizations_OpenIddictApplications_ApplicationId")
            .FromTable("OpenIddictAuthorizations").ForeignColumn("ApplicationId")
            .ToTable("OpenIddictApplications").PrimaryColumn("Id")
            .OnDelete(Rule.None);

        Create.ForeignKey("FK_OpenIddictTokens_OpenIddictApplications_ApplicationId")
            .FromTable("OpenIddictTokens").ForeignColumn("ApplicationId")
            .ToTable("OpenIddictApplications").PrimaryColumn("Id")
            .OnDelete(Rule.None);

        Create.ForeignKey("FK_OpenIddictTokens_OpenIddictAuthorizations_AuthorizationId")
            .FromTable("OpenIddictTokens").ForeignColumn("AuthorizationId")
            .ToTable("OpenIddictAuthorizations").PrimaryColumn("Id")
            .OnDelete(Rule.None);
    }

    private void CreateIndexes()
    {
        Create.Index("IX_OpenIddictApplications_ClientId").OnTable("OpenIddictApplications")
            .OnColumn("ClientId").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_OpenIddictScopes_Name").OnTable("OpenIddictScopes")
            .OnColumn("Name").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_OpenIddictAuthorizations_ApplicationId_Status_Subject_Type")
            .OnTable("OpenIddictAuthorizations")
            .OnColumn("ApplicationId").Ascending()
            .OnColumn("Status").Ascending()
            .OnColumn("Subject").Ascending()
            .OnColumn("Type").Ascending();

        Create.Index("IX_OpenIddictTokens_ApplicationId_Status_Subject_Type")
            .OnTable("OpenIddictTokens")
            .OnColumn("ApplicationId").Ascending()
            .OnColumn("Status").Ascending()
            .OnColumn("Subject").Ascending()
            .OnColumn("Type").Ascending();

        Create.Index("IX_OpenIddictTokens_AuthorizationId").OnTable("OpenIddictTokens")
            .OnColumn("AuthorizationId").Ascending();

        Create.Index("IX_OpenIddictTokens_ReferenceId").OnTable("OpenIddictTokens")
            .OnColumn("ReferenceId").Ascending()
            .WithOptions().Unique();
    }
}
