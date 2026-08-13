using System.Data;
using FluentMigrator;

namespace Rstash.Database.Migrations;

/// <summary>
/// The complete rstash metadata schema in one database-neutral migration. This
/// replaces the six provider-specific EF Core migrations: FluentMigrator now owns
/// DDL and maps each fluent type per dialect (e.g. <c>AsDateTimeOffset</c> →
/// <c>TEXT</c>/<c>timestamptz</c>/<c>datetimeoffset</c>), so the same migration
/// runs on SQLite today and Postgres/SQL Server/MySQL as they are wired.
///
/// The schema mirrors the EF Core model exactly (table/column names, lengths,
/// nullability, indexes, and the two <c>DEFAULT ''</c> columns) — EF remains the
/// runtime query + Identity layer and trusts this schema. The round-trip tests
/// guard against the two drifting apart.
/// </summary>
[Migration(202607010001, "Initial rstash schema (nodes, settings, OAuth, audit, egress, ASP.NET Identity)")]
public sealed class InitialCreate : Migration
{
    public override void Up()
    {
        CreateApplicationTables();
        CreateIdentityTables();
        CreateDataProtectionKeys();
        CreateForeignKeys();
        CreateIndexes();
    }

    public override void Down()
    {
        Delete.Table("DataProtectionKeys");

        // Dependents first (FK order), then principals.
        Delete.Table("AspNetRoleClaims");
        Delete.Table("AspNetUserClaims");
        Delete.Table("AspNetUserLogins");
        Delete.Table("AspNetUserRoles");
        Delete.Table("AspNetUserTokens");
        Delete.Table("AspNetRoles");
        Delete.Table("AspNetUsers");

        Delete.Table("audit_log");
        Delete.Table("egress_usage");
        Delete.Table("authorization_codes");
        Delete.Table("oauth_tokens");
        Delete.Table("settings");
        Delete.Table("nodes");
    }

    /// <summary>
    /// The ASP.NET Core Data Protection key ring, which encrypts the auth cookie and
    /// antiforgery tokens. Left on the default provider these live in the container
    /// filesystem (or, with no suitable location, in memory), so every restart
    /// generates a fresh ring and silently signs out every user.
    ///
    /// The shape matches <c>DataProtectionKey</c> from
    /// <c>Microsoft.AspNetCore.DataProtection.EntityFrameworkCore</c>: an int identity
    /// key plus two unbounded nullable strings. <c>Xml</c> holds the serialized key
    /// element, which is why it is not length-capped.
    /// </summary>
    private void CreateDataProtectionKeys()
    {
        Create.Table("DataProtectionKeys")
            .WithColumn("Id").AsInt32().NotNullable().PrimaryKey().Identity()
            .WithColumn("FriendlyName").AsString(int.MaxValue).Nullable()
            .WithColumn("Xml").AsString(int.MaxValue).Nullable();
    }

    private void CreateApplicationTables()
    {
        Create.Table("nodes")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            .WithColumn("UserId").AsInt64().NotNullable()
            .WithColumn("Path").AsString(512).NotNullable()
            .WithColumn("ContentType").AsString(255).NotNullable().WithDefaultValue("")
            .WithColumn("ContentLength").AsInt64().NotNullable()
            .WithColumn("ETag").AsString(255).NotNullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable()
            .WithColumn("UpdatedAt").AsDateTimeOffset().NotNullable();

        Create.Table("settings")
            .WithColumn("Key").AsString(255).NotNullable().PrimaryKey()
            .WithColumn("Value").AsString(4096).NotNullable()
            .WithColumn("UpdatedAt").AsDateTimeOffset().NotNullable();

        Create.Table("oauth_tokens")
            .WithColumn("Token").AsString(255).NotNullable().PrimaryKey()
            .WithColumn("UserId").AsInt64().NotNullable()
            .WithColumn("ClientId").AsString(255).NotNullable()
            .WithColumn("Scopes").AsString(1024).NotNullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable()
            .WithColumn("ExpiresAt").AsDateTimeOffset().Nullable()
            .WithColumn("RefreshToken").AsString(255).Nullable()
            .WithColumn("RefreshExpiresAt").AsDateTimeOffset().Nullable();

        Create.Index("IX_oauth_tokens_RefreshToken")
            .OnTable("oauth_tokens").OnColumn("RefreshToken").Ascending();

        Create.Table("authorization_codes")
            .WithColumn("Code").AsString(255).NotNullable().PrimaryKey()
            .WithColumn("UserId").AsInt64().NotNullable()
            .WithColumn("ClientId").AsString(255).NotNullable()
            .WithColumn("RedirectUri").AsString(2048).NotNullable()
            .WithColumn("Scopes").AsString(1024).NotNullable()
            .WithColumn("CodeChallenge").AsString(255).NotNullable()
            .WithColumn("CodeChallengeMethod").AsString(32).NotNullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable()
            .WithColumn("ExpiresAt").AsDateTimeOffset().NotNullable()
            .WithColumn("Used").AsBoolean().NotNullable();

        Create.Table("egress_usage")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            .WithColumn("UserId").AsInt64().NotNullable()
            .WithColumn("Period").AsString(7).NotNullable()
            .WithColumn("BytesOut").AsInt64().NotNullable()
            .WithColumn("UpdatedAt").AsDateTimeOffset().NotNullable();

        Create.Table("audit_log")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            .WithColumn("ActorId").AsInt64().NotNullable()
            .WithColumn("Action").AsString(255).NotNullable()
            .WithColumn("TargetType").AsString(255).NotNullable()
            .WithColumn("TargetId").AsString(255).NotNullable()
            .WithColumn("Details").AsString(4096).NotNullable().WithDefaultValue("")
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable();
    }

    private void CreateIdentityTables()
    {
        Create.Table("AspNetRoles")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            .WithColumn("Name").AsString(256).Nullable()
            .WithColumn("NormalizedName").AsString(256).Nullable()
            .WithColumn("ConcurrencyStamp").AsString(int.MaxValue).Nullable();

        Create.Table("AspNetUsers")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            // rstash-specific columns.
            .WithColumn("IsAdmin").AsBoolean().NotNullable()
            .WithColumn("StorageQuota").AsInt64().NotNullable()
            .WithColumn("EgressQuota").AsInt64().NotNullable()
            .WithColumn("Disabled").AsBoolean().NotNullable()
            .WithColumn("Approved").AsBoolean().NotNullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable()
            .WithColumn("LastLoginAt").AsDateTimeOffset().Nullable()
            .WithColumn("LastLoginIp").AsString(int.MaxValue).Nullable()
            // Standard ASP.NET Identity columns.
            .WithColumn("UserName").AsString(256).Nullable()
            .WithColumn("NormalizedUserName").AsString(256).Nullable()
            .WithColumn("Email").AsString(256).Nullable()
            .WithColumn("NormalizedEmail").AsString(256).Nullable()
            .WithColumn("EmailConfirmed").AsBoolean().NotNullable()
            .WithColumn("PasswordHash").AsString(int.MaxValue).Nullable()
            .WithColumn("SecurityStamp").AsString(int.MaxValue).Nullable()
            .WithColumn("ConcurrencyStamp").AsString(int.MaxValue).Nullable()
            .WithColumn("PhoneNumber").AsString(int.MaxValue).Nullable()
            .WithColumn("PhoneNumberConfirmed").AsBoolean().NotNullable()
            .WithColumn("TwoFactorEnabled").AsBoolean().NotNullable()
            .WithColumn("LockoutEnd").AsDateTimeOffset().Nullable()
            .WithColumn("LockoutEnabled").AsBoolean().NotNullable()
            .WithColumn("AccessFailedCount").AsInt32().NotNullable();

        Create.Table("AspNetRoleClaims")
            .WithColumn("Id").AsInt32().NotNullable().PrimaryKey().Identity()
            .WithColumn("RoleId").AsInt64().NotNullable()
            .WithColumn("ClaimType").AsString(int.MaxValue).Nullable()
            .WithColumn("ClaimValue").AsString(int.MaxValue).Nullable();

        Create.Table("AspNetUserClaims")
            .WithColumn("Id").AsInt32().NotNullable().PrimaryKey().Identity()
            .WithColumn("UserId").AsInt64().NotNullable()
            .WithColumn("ClaimType").AsString(int.MaxValue).Nullable()
            .WithColumn("ClaimValue").AsString(int.MaxValue).Nullable();

        Create.Table("AspNetUserLogins")
            .WithColumn("LoginProvider").AsString(int.MaxValue).NotNullable().PrimaryKey()
            .WithColumn("ProviderKey").AsString(int.MaxValue).NotNullable().PrimaryKey()
            .WithColumn("ProviderDisplayName").AsString(int.MaxValue).Nullable()
            .WithColumn("UserId").AsInt64().NotNullable();

        Create.Table("AspNetUserRoles")
            .WithColumn("UserId").AsInt64().NotNullable().PrimaryKey()
            .WithColumn("RoleId").AsInt64().NotNullable().PrimaryKey();

        Create.Table("AspNetUserTokens")
            .WithColumn("UserId").AsInt64().NotNullable().PrimaryKey()
            .WithColumn("LoginProvider").AsString(int.MaxValue).NotNullable().PrimaryKey()
            .WithColumn("Name").AsString(int.MaxValue).NotNullable().PrimaryKey()
            .WithColumn("Value").AsString(int.MaxValue).Nullable();
    }

    private void CreateForeignKeys()
    {
        Create.ForeignKey("FK_AspNetRoleClaims_AspNetRoles_RoleId")
            .FromTable("AspNetRoleClaims").ForeignColumn("RoleId")
            .ToTable("AspNetRoles").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);

        Create.ForeignKey("FK_AspNetUserClaims_AspNetUsers_UserId")
            .FromTable("AspNetUserClaims").ForeignColumn("UserId")
            .ToTable("AspNetUsers").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);

        Create.ForeignKey("FK_AspNetUserLogins_AspNetUsers_UserId")
            .FromTable("AspNetUserLogins").ForeignColumn("UserId")
            .ToTable("AspNetUsers").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);

        Create.ForeignKey("FK_AspNetUserRoles_AspNetRoles_RoleId")
            .FromTable("AspNetUserRoles").ForeignColumn("RoleId")
            .ToTable("AspNetRoles").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);

        Create.ForeignKey("FK_AspNetUserRoles_AspNetUsers_UserId")
            .FromTable("AspNetUserRoles").ForeignColumn("UserId")
            .ToTable("AspNetUsers").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);

        Create.ForeignKey("FK_AspNetUserTokens_AspNetUsers_UserId")
            .FromTable("AspNetUserTokens").ForeignColumn("UserId")
            .ToTable("AspNetUsers").PrimaryColumn("Id")
            .OnDelete(Rule.Cascade);
    }

    private void CreateIndexes()
    {
        // Application tables.
        Create.Index("IX_nodes_UserId_Path").OnTable("nodes")
            .OnColumn("UserId").Ascending()
            .OnColumn("Path").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_oauth_tokens_UserId").OnTable("oauth_tokens")
            .OnColumn("UserId").Ascending();

        Create.Index("IX_egress_usage_UserId_Period").OnTable("egress_usage")
            .OnColumn("UserId").Ascending()
            .OnColumn("Period").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_audit_log_CreatedAt").OnTable("audit_log")
            .OnColumn("CreatedAt").Ascending();

        // ASP.NET Identity indexes (names match the EF-generated schema).
        Create.Index("RoleNameIndex").OnTable("AspNetRoles")
            .OnColumn("NormalizedName").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_AspNetRoleClaims_RoleId").OnTable("AspNetRoleClaims")
            .OnColumn("RoleId").Ascending();

        Create.Index("IX_AspNetUserClaims_UserId").OnTable("AspNetUserClaims")
            .OnColumn("UserId").Ascending();

        Create.Index("IX_AspNetUserLogins_UserId").OnTable("AspNetUserLogins")
            .OnColumn("UserId").Ascending();

        Create.Index("IX_AspNetUserRoles_RoleId").OnTable("AspNetUserRoles")
            .OnColumn("RoleId").Ascending();

        Create.Index("EmailIndex").OnTable("AspNetUsers")
            .OnColumn("NormalizedEmail").Ascending();

        Create.Index("UserNameIndex").OnTable("AspNetUsers")
            .OnColumn("NormalizedUserName").Ascending()
            .WithOptions().Unique();
    }
}
