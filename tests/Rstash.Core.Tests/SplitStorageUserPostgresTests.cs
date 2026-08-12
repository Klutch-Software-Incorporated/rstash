using Npgsql;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// The Postgres half of <see cref="SplitStorageUserMigrationTests"/>. That one seeds
/// rows and applies the split on SQLite, and its own comments name Postgres as the
/// dialect the sequence fix-up exists for — but it never runs there, so the migration
/// shipped unable to apply to a Postgres database at all.
/// </summary>
/// <remarks>
/// Everything here is quoted, because that is the bug: FluentMigrator builds the
/// schema through its fluent API, which quotes, so the identifiers really are
/// mixed-case and unquoted SQL silently asks for lower-case names that do not exist.
/// The seeds need OVERRIDING SYSTEM VALUE for the same reason the migration does —
/// the identity columns are GENERATED ALWAYS.
/// </remarks>
public sealed class SplitStorageUserPostgresTests : IDisposable
{
    /// <summary>The migration immediately before <c>SplitStorageUser</c>.</summary>
    private const long BeforeSplit = 202607280002;

    private const long Split = 202607280003;

    private readonly string _dsn;

    public SplitStorageUserPostgresTests()
    {
        // Not reached when the fact is skipped (xUnit doesn't construct the class).
        _dsn = PostgresServer.CreateScratchDatabase();
    }

    public void Dispose() => PostgresServer.DropScratchDatabase(_dsn);

    [PostgresFact]
    public void Backfill_CarriesExistingAccountsAcross_PreservingIdsAndLimits()
    {
        SchemaMigrator.MigrateUp(_dsn, BeforeSplit);

        Execute(
            """
            INSERT INTO "AspNetUsers"
                ("Id", "IsAdmin", "StorageQuota", "EgressQuota", "Disabled", "Approved",
                 "ExternallyManaged", "CreatedAt", "UserName", "NormalizedUserName",
                 "EmailConfirmed", "PhoneNumberConfirmed", "TwoFactorEnabled",
                 "LockoutEnabled", "AccessFailedCount")
            OVERRIDING SYSTEM VALUE
            VALUES
                (7, FALSE, 1024, 2048, FALSE, TRUE, FALSE, '2026-01-01T00:00:00+00:00',
                 'Alice', 'ALICE', FALSE, FALSE, FALSE, FALSE, 0),
                (9, FALSE, 0, 0, TRUE, TRUE, FALSE, '2026-01-02T00:00:00+00:00',
                 'bob', 'BOB', FALSE, FALSE, FALSE, FALSE, 0);

            INSERT INTO nodes ("UserId", "Path", "ContentType", "ContentLength", "ETag", "CreatedAt", "UpdatedAt")
            VALUES
                (7, 'documents/a.txt', 'text/plain', 5, 'etag-a', '2026-01-01T00:00:00+00:00', '2026-01-01T00:00:00+00:00'),
                (9, 'documents/b.txt', 'text/plain', 5, 'etag-b', '2026-01-02T00:00:00+00:00', '2026-01-02T00:00:00+00:00');
            """);

        SchemaMigrator.MigrateUp(_dsn, Split);

        // Ids carry over unchanged — that is what keeps nodes.UserId valid without
        // rewriting a single row.
        Assert.Equal(
            "7|1024|2048|false|Alice|ALICE|7",
            Scalar("""
                SELECT "Id" || '|' || "MaxStorage" || '|' || "MaxEgress" || '|' || "Disabled"
                       || '|' || "UserName" || '|' || "NormalizedUserName" || '|' || "Subject"
                FROM storage_users WHERE "Id" = 7;
                """));

        // The subject is the id as text. A CHAR(n) cast would blank-pad it to the full
        // width on Postgres, which would not match the sub claim the provider issues.
        Assert.Equal("9", Scalar("SELECT \"Subject\" FROM storage_users WHERE \"Id\" = 9;"));
        // ::text so the comparison is against Postgres's own rendering rather than
        // Npgsql's .NET bool ("True"), matching the concatenated assertion above.
        Assert.Equal("true", Scalar("SELECT \"Disabled\"::text FROM storage_users WHERE \"Id\" = 9;"));

        // Every node still resolves to an owner.
        Assert.Equal(
            "0",
            Scalar("""
                SELECT COUNT(*) FROM nodes n
                WHERE NOT EXISTS (SELECT 1 FROM storage_users s WHERE s."Id" = n."UserId");
                """));

        // The provider-owned columns are gone from the identity row; Approved stays,
        // because approval is a local registration gate rather than an entitlement.
        var userColumns = Scalar(
            """
            SELECT string_agg(column_name, ',')
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'AspNetUsers';
            """);
        foreach (var dropped in new[] { "StorageQuota", "EgressQuota", "Disabled", "ExternallyManaged" })
        {
            Assert.DoesNotContain(dropped, userColumns, StringComparison.Ordinal);
        }

        Assert.Contains("Approved", userColumns, StringComparison.Ordinal);
    }

    [PostgresFact]
    public void NewRowsAfterBackfill_DoNotCollideWithSeededIds()
    {
        SchemaMigrator.MigrateUp(_dsn, BeforeSplit);

        Execute(
            """
            INSERT INTO "AspNetUsers"
                ("Id", "IsAdmin", "StorageQuota", "EgressQuota", "Disabled", "Approved",
                 "ExternallyManaged", "CreatedAt", "UserName", "NormalizedUserName",
                 "EmailConfirmed", "PhoneNumberConfirmed", "TwoFactorEnabled",
                 "LockoutEnabled", "AccessFailedCount")
            OVERRIDING SYSTEM VALUE
            VALUES (42, FALSE, 0, 0, FALSE, TRUE, FALSE, '2026-01-01T00:00:00+00:00',
                    'carol', 'CAROL', FALSE, FALSE, FALSE, FALSE, 0);
            """);

        SchemaMigrator.MigrateUp(_dsn, Split);

        // This is the assertion the SQLite test cannot make. Postgres does not advance
        // an identity sequence for explicitly-supplied keys, so without the migration's
        // setval the next insert reuses id 1 and collides with a backfilled account.
        Execute(
            """
            INSERT INTO storage_users
                ("Subject", "UserName", "NormalizedUserName", "Plan", "MaxStorage",
                 "MaxEgress", "Disabled", "CreatedAt")
            VALUES ('sub-new', 'dave', 'DAVE', '', 0, 0, FALSE, '2026-07-28T00:00:00+00:00');
            """);

        var newId = long.Parse(
            Scalar("SELECT \"Id\" FROM storage_users WHERE \"NormalizedUserName\" = 'DAVE';")!,
            System.Globalization.CultureInfo.InvariantCulture);

        Assert.True(newId > 42, $"new row got id {newId}, which collides with the backfilled range");
    }

    private void Execute(string sql)
    {
        using var connection = new NpgsqlConnection(_dsn["postgres:".Length..]);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        command.ExecuteNonQuery();
    }

    private string? Scalar(string sql)
    {
        using var connection = new NpgsqlConnection(_dsn["postgres:".Length..]);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        return command.ExecuteScalar()?.ToString();
    }
}
