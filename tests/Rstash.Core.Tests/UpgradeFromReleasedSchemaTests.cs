using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>
/// The actual upgrade path a deployed server takes: a database sitting at the
/// released schema, holding real accounts and documents, brought all the way up in
/// one startup.
/// </summary>
/// <remarks>
/// Every other migration test either starts from empty or stops at an intermediate
/// version, so none of them exercises what production will actually do. This one
/// seeds at <c>InitialCreate</c> — the only migration that shipped — and then runs
/// the full set.
/// </remarks>
public sealed class UpgradeFromReleasedSchemaTests : IDisposable
{
    /// <summary>The schema currently deployed: everything else is on this branch.</summary>
    private const long ReleasedSchema = 202607010001;

    private readonly string _path =
        Path.Combine(Path.GetTempPath(), $"rstash-upgrade-{Guid.NewGuid():N}.sqlite");

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
    public async Task DeployedDatabaseWithData_UpgradesInOneStep_WithoutLosingAnything()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}", ReleasedSchema);

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        // An account with a quota and documents, plus a token and an audit entry —
        // every table that keys off the user id the split has to preserve.
        Execute(
            connection,
            """
            INSERT INTO AspNetUsers
                (Id, IsAdmin, StorageQuota, EgressQuota, Disabled, Approved, ExternallyManaged,
                 CreatedAt, UserName, NormalizedUserName, Email, NormalizedEmail,
                 EmailConfirmed, PhoneNumberConfirmed, TwoFactorEnabled, LockoutEnabled,
                 AccessFailedCount, PasswordHash)
            VALUES (3, 1, 5368709120, 1073741824, 0, 1, 0, '2026-05-01T00:00:00+00:00',
                    'curtis', 'CURTIS', 'c@example.com', 'C@EXAMPLE.COM',
                    1, 0, 0, 1, 0, 'AQAAAAIAAYag-not-a-real-hash');

            INSERT INTO nodes (UserId, Path, ContentType, ContentLength, ETag, CreatedAt, UpdatedAt)
            VALUES (3, 'documents/notes.txt', 'text/plain', 11, 'etag-1',
                    '2026-05-01T00:00:00+00:00', '2026-05-01T00:00:00+00:00');

            INSERT INTO oauth_tokens (Token, UserId, ClientId, Scopes, CreatedAt)
            VALUES ('tok-abc', 3, 'https://litewrite.net', 'documents:rw', '2026-05-01T00:00:00+00:00');

            INSERT INTO audit_log (ActorId, Action, TargetType, TargetId, Details, CreatedAt)
            VALUES (3, 'storage.put', 'storage', '/documents/notes.txt', '', '2026-05-01T00:00:00+00:00');

            INSERT INTO egress_usage (UserId, Period, BytesOut, UpdatedAt)
            VALUES (3, '2026-05', 4096, '2026-05-01T00:00:00+00:00');
            """);

        // The whole remaining migration set, exactly as a deploy would apply it.
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        // The account survived, with its limits carried onto the storage record.
        Assert.Equal(
            "3|curtis|CURTIS|3|5368709120|1073741824|0",
            Scalar(connection, """
                SELECT Id || '|' || UserName || '|' || NormalizedUserName || '|' || Subject
                       || '|' || MaxStorage || '|' || MaxEgress || '|' || Disabled
                FROM storage_users;
                """));

        // Identity keeps the credential half.
        Assert.Equal(
            "curtis|1|1",
            Scalar(connection, "SELECT UserName || '|' || IsAdmin || '|' || Approved FROM AspNetUsers;"));

        // Nothing that keys off the user id was orphaned.
        foreach (var (table, column) in new[]
                 {
                     ("nodes", "UserId"), ("oauth_tokens", "UserId"),
                     ("audit_log", "ActorId"), ("egress_usage", "UserId"),
                 })
        {
            Assert.Equal(
                "0",
                Scalar(connection, $"""
                    SELECT COUNT(*) FROM {table} t
                    WHERE NOT EXISTS (SELECT 1 FROM storage_users s WHERE s.Id = t.{column});
                    """));
        }

        // The document is still readable through EF against the upgraded schema.
        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase($"sqlite:{_path}")
            .Options;
        await using var db = new RstashDbContext(options);

        var node = await db.Nodes.SingleAsync();
        Assert.Equal("documents/notes.txt", node.Path);
        Assert.Equal(11, node.ContentLength);

        // And the storage token still resolves to an owner that exists.
        var token = await db.OAuthTokens.SingleAsync();
        Assert.Equal(3, token.UserId);
        Assert.True(await db.StorageUsers.AnyAsync(s => s.Id == token.UserId));
    }

    [Fact]
    public void UpgradeIsIdempotent_ASecondStartupChangesNothing()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}", ReleasedSchema);
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();
        var applied = Scalar(connection, "SELECT COUNT(*) FROM VersionInfo;");

        // A restart re-runs MigrateUp; it must be a no-op rather than an error.
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        Assert.Equal(applied, Scalar(connection, "SELECT COUNT(*) FROM VersionInfo;"));
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
