using Microsoft.Data.Sqlite;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// The split is a data migration, not just a shape change: it has to carry existing
/// accounts across without breaking the foreign keys that point at them. Every other
/// test builds its schema from scratch, where the backfill has nothing to do — so
/// this one stops at the previous version, seeds rows, and then applies the split.
/// </summary>
public sealed class SplitStorageUserMigrationTests : IDisposable
{
    /// <summary>The migration immediately before <c>SplitStorageUser</c>.</summary>
    private const long BeforeSplit = 202607280002;

    private const long Split = 202607280003;

    private readonly string _path =
        Path.Combine(Path.GetTempPath(), $"rstash-split-{Guid.NewGuid():N}.sqlite");

    public void Dispose()
    {
        SqliteConnection.ClearAllPools();
        foreach (var p in new[] { _path, _path + "-wal", _path + "-shm" })
        {
            try
            {
                File.Delete(p);
            }
            catch (IOException)
            {
            }
        }
    }

    [Fact]
    public void Backfill_CarriesExistingAccountsAcross_PreservingIdsAndLimits()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}", BeforeSplit);

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        // Two accounts with non-adjacent ids, one of them disabled and quota-capped,
        // plus a node owned by each so the foreign keys have something to preserve.
        Execute(
            connection,
            """
            INSERT INTO AspNetUsers
                (Id, IsAdmin, StorageQuota, EgressQuota, Disabled, Approved, ExternallyManaged,
                 CreatedAt, UserName, NormalizedUserName, EmailConfirmed, PhoneNumberConfirmed,
                 TwoFactorEnabled, LockoutEnabled, AccessFailedCount)
            VALUES
                (7, 0, 1024, 2048, 0, 1, 0, '2026-01-01T00:00:00+00:00', 'Alice', 'ALICE', 0, 0, 0, 0, 0),
                (9, 0, 0, 0, 1, 1, 0, '2026-01-02T00:00:00+00:00', 'bob', 'BOB', 0, 0, 0, 0, 0);

            INSERT INTO nodes (UserId, Path, ContentType, ContentLength, ETag, CreatedAt, UpdatedAt)
            VALUES
                (7, 'documents/a.txt', 'text/plain', 5, 'etag-a', '2026-01-01T00:00:00+00:00', '2026-01-01T00:00:00+00:00'),
                (9, 'documents/b.txt', 'text/plain', 5, 'etag-b', '2026-01-02T00:00:00+00:00', '2026-01-02T00:00:00+00:00');
            """);

        SchemaMigrator.MigrateUp($"sqlite:{_path}", Split);

        // Ids carry over unchanged — that is what keeps nodes.UserId valid without
        // rewriting a single row.
        Assert.Equal(
            "7|1024|2048|0|Alice|ALICE|7",
            Scalar(connection, """
                SELECT Id || '|' || MaxStorage || '|' || MaxEgress || '|' || Disabled
                       || '|' || UserName || '|' || NormalizedUserName || '|' || Subject
                FROM storage_users WHERE Id = 7;
                """));

        Assert.Equal("1", Scalar(connection, "SELECT Disabled FROM storage_users WHERE Id = 9;"));

        // Every node still resolves to an owner.
        Assert.Equal(
            "0",
            Scalar(connection, """
                SELECT COUNT(*) FROM nodes n
                WHERE NOT EXISTS (SELECT 1 FROM storage_users s WHERE s.Id = n.UserId);
                """));

        // The provider-owned columns are gone from the identity row.
        var userColumns = Scalar(connection, "SELECT GROUP_CONCAT(name) FROM pragma_table_info('AspNetUsers');");
        foreach (var dropped in new[] { "StorageQuota", "EgressQuota", "Disabled", "ExternallyManaged" })
        {
            Assert.DoesNotContain(dropped, userColumns, StringComparison.Ordinal);
        }

        // Approved stays: it is a local registration gate, not an entitlement.
        Assert.Contains("Approved", userColumns, StringComparison.Ordinal);
    }

    [Fact]
    public void NewRowsAfterBackfill_DoNotCollideWithSeededIds()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}", BeforeSplit);

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        Execute(
            connection,
            """
            INSERT INTO AspNetUsers
                (Id, IsAdmin, StorageQuota, EgressQuota, Disabled, Approved, ExternallyManaged,
                 CreatedAt, UserName, NormalizedUserName, EmailConfirmed, PhoneNumberConfirmed,
                 TwoFactorEnabled, LockoutEnabled, AccessFailedCount)
            VALUES (42, 0, 0, 0, 0, 1, 0, '2026-01-01T00:00:00+00:00', 'carol', 'CAROL', 0, 0, 0, 0, 0);
            """);

        SchemaMigrator.MigrateUp($"sqlite:{_path}", Split);

        // The identity column has to resume past the highest backfilled id. SQLite
        // tracks that itself; Postgres needs the setval the migration performs, which
        // is the one place this schema cannot stay dialect-agnostic.
        Execute(
            connection,
            """
            INSERT INTO storage_users
                (Subject, UserName, NormalizedUserName, Plan, MaxStorage, MaxEgress, Disabled, CreatedAt)
            VALUES ('sub-new', 'dave', 'DAVE', '', 0, 0, 0, '2026-07-28T00:00:00+00:00');
            """);

        var newId = long.Parse(
            Scalar(connection, "SELECT Id FROM storage_users WHERE NormalizedUserName = 'DAVE';")!,
            System.Globalization.CultureInfo.InvariantCulture);

        Assert.True(newId > 42, $"new row got id {newId}, which collides with the backfilled range");
    }

    private static void Execute(SqliteConnection connection, string sql)
    {
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        command.ExecuteNonQuery();
    }

    private static string? Scalar(SqliteConnection connection, string sql)
    {
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        return command.ExecuteScalar()?.ToString();
    }
}
