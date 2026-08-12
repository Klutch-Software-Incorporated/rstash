using FluentMigrator;

namespace Rstash.Database.Migrations;

/// <summary>
/// Moves the storage server's user record out of the Identity table.
///
/// <c>AspNetUsers</c> held both credentials and entitlements — password hash next to
/// storage quota. That works only while one deployment owns both. The moment
/// accounts come from an external provider, the identity half of the table goes away
/// and every storage query that touched a provider-owned column becomes a migration.
/// Splitting now means the bundled and external providers run the same storage code.
///
/// The backfill copies <c>AspNetUsers.Id</c> into <c>storage_users.Id</c>, so
/// <c>nodes.UserId</c>, <c>oauth_tokens.UserId</c>, <c>audit_log.ActorId</c>, and
/// <c>egress_usage.UserId</c> keep pointing at valid rows without rewriting a single
/// one. Nothing should rely on the two staying equal afterwards — new rows get their
/// own identity values.
/// </summary>
[Migration(202607280003, "Split storage_users out of AspNetUsers")]
public sealed class SplitStorageUser : Migration
{
    /// <summary>
    /// Matches every FluentMigrator processor id for PostgreSQL. The runner reports a
    /// version-specific id, so an equality test silently matches nothing.
    /// </summary>
    private static bool IsPostgres(string dialect) =>
        dialect.StartsWith("Postgres", StringComparison.OrdinalIgnoreCase);

    public override void Up()
    {
        Create.Table("storage_users")
            .WithColumn("Id").AsInt64().NotNullable().PrimaryKey().Identity()
            .WithColumn("Subject").AsString(255).NotNullable()
            .WithColumn("UserName").AsString(255).NotNullable()
            .WithColumn("NormalizedUserName").AsString(255).NotNullable()
            .WithColumn("Plan").AsString(64).NotNullable().WithDefaultValue("")
            .WithColumn("MaxStorage").AsInt64().NotNullable().WithDefaultValue(0)
            .WithColumn("MaxEgress").AsInt64().NotNullable().WithDefaultValue(0)
            .WithColumn("RateLimitOverride").AsInt32().Nullable()
            .WithColumn("Disabled").AsBoolean().NotNullable().WithDefaultValue(false)
            .WithColumn("SyncedAt").AsDateTimeOffset().Nullable()
            .WithColumn("SourceIssuer").AsString(255).Nullable()
            .WithColumn("CreatedAt").AsDateTimeOffset().NotNullable();

        Create.Index("IX_storage_users_Subject").OnTable("storage_users")
            .OnColumn("Subject").Ascending()
            .WithOptions().Unique();

        Create.Index("IX_storage_users_NormalizedUserName").OnTable("storage_users")
            .OnColumn("NormalizedUserName").Ascending()
            .WithOptions().Unique();

        // Subject is the bundled provider's local user id, which is what
        // /connect/authorize puts in the sub claim. NormalizedUserName is copied from
        // Identity's own normalized column rather than recomputed, so the two agree
        // even for names whose upper-casing is culture-sensitive.
        //
        // Every identifier is quoted. The rest of the schema is built through
        // FluentMigrator's fluent API, which quotes, so the tables and columns really
        // are mixed-case; Postgres folds an *unquoted* identifier to lower case, so
        // bare `AspNetUsers` names a relation that does not exist. VARCHAR rather than
        // CHAR for the subject cast, because CHAR(n) blank-pads to the full width on
        // Postgres and the padded value would not match the sub claim.
        const string insertInto =
            """
            INSERT INTO storage_users
                ("Id", "Subject", "UserName", "NormalizedUserName", "Plan",
                 "MaxStorage", "MaxEgress", "Disabled", "CreatedAt")
            """;
        const string selectFrom =
            """
            SELECT "Id", CAST("Id" AS VARCHAR(255)), "UserName", "NormalizedUserName", '',
                   "StorageQuota", "EgressQuota", "Disabled", "CreatedAt"
            FROM "AspNetUsers";
            """;

        // Postgres declares the identity column GENERATED ALWAYS and rejects an
        // explicit key outright, so its backfill has to say OVERRIDING SYSTEM VALUE.
        // Both branches test the same predicate so they cannot both run — or, worse,
        // both be skipped, which is how an earlier version of this migration reported
        // success while copying zero rows.
        IfDatabase(IsPostgres).Execute.Sql($"{insertInto}\nOVERRIDING SYSTEM VALUE\n{selectFrom}");
        IfDatabase(dialect => !IsPostgres(dialect)).Execute.Sql($"{insertInto}\n{selectFrom}");

        // Postgres does not advance an identity sequence for explicitly-supplied keys,
        // so the next insert would collide with a backfilled row. SQLite tracks the
        // high-water mark itself and needs nothing. This is the one place the schema
        // cannot stay dialect-agnostic; it is a sequence fix-up, not a shape difference.
        //
        // pg_get_serial_sequence treats its *column* argument as a quoted identifier
        // and preserves case, so this has to be 'Id': the lower-case form returns NULL
        // and setval then fails. The table argument may name a schema, so it is folded
        // instead — lower case is right there.
        IfDatabase(IsPostgres).Execute.Sql(
            """
            SELECT setval(
                pg_get_serial_sequence('storage_users', 'Id'),
                COALESCE((SELECT MAX("Id") FROM storage_users), 1),
                true);
            """);

        // Provider-owned columns leave the identity row. Approved and IsAdmin stay:
        // approval is a local registration gate, not an entitlement.
        Delete.Column("StorageQuota").FromTable("AspNetUsers");
        Delete.Column("EgressQuota").FromTable("AspNetUsers");
        Delete.Column("Disabled").FromTable("AspNetUsers");
        Delete.Column("ExternallyManaged").FromTable("AspNetUsers");
    }

    public override void Down()
    {
        Alter.Table("AspNetUsers")
            .AddColumn("StorageQuota").AsInt64().NotNullable().WithDefaultValue(0)
            .AddColumn("EgressQuota").AsInt64().NotNullable().WithDefaultValue(0)
            .AddColumn("Disabled").AsBoolean().NotNullable().WithDefaultValue(false)
            .AddColumn("ExternallyManaged").AsBoolean().NotNullable().WithDefaultValue(false);

        // Quoted for the same reason as Up. The Disabled fallback is a real boolean
        // rather than 0, which Postgres rejects for a boolean column.
        Execute.Sql(
            """
            UPDATE "AspNetUsers" SET
                "StorageQuota" = COALESCE(
                    (SELECT s."MaxStorage" FROM storage_users s WHERE s."Id" = "AspNetUsers"."Id"), 0),
                "EgressQuota" = COALESCE(
                    (SELECT s."MaxEgress" FROM storage_users s WHERE s."Id" = "AspNetUsers"."Id"), 0),
                "Disabled" = COALESCE(
                    (SELECT s."Disabled" FROM storage_users s WHERE s."Id" = "AspNetUsers"."Id"), FALSE);
            """);

        Delete.Table("storage_users");
    }
}
