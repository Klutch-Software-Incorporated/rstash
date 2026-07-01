using Microsoft.EntityFrameworkCore;
using Npgsql;
using Rstash.Database;
using Rstash.Model;
using Rstash.Storage;

namespace Rstash.Core.Tests;

/// <summary>
/// Proves the FluentMigrator schema and the EF/blob stores work end-to-end on a real
/// PostgreSQL server — the Postgres analog of <see cref="SchemaMigratorTests"/> and
/// <see cref="DatabaseStorageTests"/>. Self-skips (via <see cref="PostgresFactAttribute"/>)
/// when no server is reachable; each test runs against its own scratch database.
/// </summary>
public sealed class PostgresProviderTests : IDisposable
{
    private readonly string _dsn;

    public PostgresProviderTests()
    {
        // Not reached when the fact is skipped (xUnit doesn't construct the class).
        _dsn = PostgresServer.CreateScratchDatabase();
    }

    public void Dispose() => PostgresServer.DropScratchDatabase(_dsn);

    [PostgresFact]
    public void MigrateUp_IsIdempotent_AndRecordsVersion()
    {
        SchemaMigrator.MigrateUp(_dsn);
        SchemaMigrator.MigrateUp(_dsn); // second run is a no-op, not an error.

        Assert.Equal(
            1L,
            (long)Scalar("SELECT COUNT(*) FROM \"VersionInfo\" WHERE \"Version\" = 202607010001;")!);
    }

    [PostgresFact]
    public void MigratedSchema_HasExpectedTablesAndUniqueIndexes()
    {
        SchemaMigrator.MigrateUp(_dsn);

        var tables = Query(
            "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public';");
        string[] expectedTables =
        [
            "nodes", "settings", "oauth_tokens", "authorization_codes", "egress_usage", "audit_log",
            "AspNetUsers", "AspNetRoles", "AspNetUserRoles", "AspNetUserClaims",
            "AspNetUserLogins", "AspNetRoleClaims", "AspNetUserTokens",
        ];
        foreach (var table in expectedTables)
        {
            Assert.Contains(table, tables);
        }

        // The named unique indexes must exist and actually be unique on Postgres too.
        var uniqueIndexes = Query(
            """
            SELECT c.relname
            FROM pg_index ix
            JOIN pg_class c ON c.oid = ix.indexrelid
            JOIN pg_class t ON t.oid = ix.indrelid
            JOIN pg_namespace n ON n.oid = t.relnamespace
            WHERE n.nspname = 'public' AND ix.indisunique;
            """);
        foreach (var index in new[]
                 {
                     "IX_nodes_UserId_Path", "IX_egress_usage_UserId_Period", "UserNameIndex", "RoleNameIndex",
                 })
        {
            Assert.Contains(index, uniqueIndexes);
        }

        // FKs must be real/enforced on Postgres (LOOSE mode is SQLite-only).
        var foreignKeys = Query(
            "SELECT constraint_name FROM information_schema.table_constraints " +
            "WHERE table_schema = 'public' AND constraint_type = 'FOREIGN KEY';");
        Assert.Contains("FK_AspNetUserRoles_AspNetUsers_UserId", foreignKeys);
    }

    [PostgresFact]
    public async Task EfModel_RoundTripsAgainstMigratedSchema()
    {
        SchemaMigrator.MigrateUp(_dsn);

        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase(_dsn)
            .Options;

        await using (var ctx = new RstashDbContext(options))
        {
            ctx.Users.Add(new ApplicationUser
            {
                UserName = "alice",
                NormalizedUserName = "ALICE",
                Email = "alice@example.com",
                NormalizedEmail = "ALICE@EXAMPLE.COM",
                CreatedAt = DateTimeOffset.UnixEpoch,
                StorageQuota = 1024,
            });

            ctx.Nodes.Add(new Node
            {
                UserId = 1,
                Path = "documents/notes.txt",
                ContentLength = 5,
                ETag = ETag.ForDocument("hello"u8),
                CreatedAt = DateTimeOffset.UnixEpoch,
                UpdatedAt = DateTimeOffset.UnixEpoch,
            });

            ctx.Settings.Add(new Setting
            {
                Key = "registration_mode", Value = "open", UpdatedAt = DateTimeOffset.UnixEpoch,
            });

            ctx.AuditLog.Add(new AuditEntry
            {
                ActorId = 1,
                Action = "put",
                TargetType = "node",
                TargetId = "documents/notes.txt",
                CreatedAt = DateTimeOffset.UnixEpoch,
            });

            await ctx.SaveChangesAsync();
        }

        await using (var ctx = new RstashDbContext(options))
        {
            var node = await ctx.Nodes.SingleAsync();
            Assert.Equal("documents/notes.txt", node.Path);
            Assert.Equal("", node.ContentType); // DEFAULT '' applied by the migration.
            Assert.Equal(DateTimeOffset.UnixEpoch, node.CreatedAt); // timestamptz round-trips as UTC.

            var user = await ctx.Users.SingleAsync();
            Assert.Equal("alice", user.UserName);
            Assert.True(user.Id > 0); // serial identity assigned on insert.

            Assert.Equal("", (await ctx.AuditLog.SingleAsync()).Details);
            Assert.Equal("open", (await ctx.Settings.SingleAsync()).Value);
        }
    }

    [PostgresFact]
    public async Task DatabaseStorage_RoundTripsAndIsCaseSensitive()
    {
        await using var store = new DatabaseStorage(_dsn);

        await store.PutAsync(1, "photos/a.jpg", "lower"u8.ToArray());
        await store.PutAsync(1, "photos/A.jpg", "upper"u8.ToArray());

        // Case-sensitive keys are distinct rows (Postgres LIKE/= is case-sensitive).
        Assert.Equal("lower"u8.ToArray(), await ReadAllAsync(store.GetAsync(1, "photos/a.jpg")));
        Assert.Equal("upper"u8.ToArray(), await ReadAllAsync(store.GetAsync(1, "photos/A.jpg")));

        await store.PutAsync(1, "Photos/keep.txt", "keep"u8.ToArray()); // different case prefix.
        await store.DeleteTreeAsync(1, "photos/");

        await Assert.ThrowsAnyAsync<IOException>(async () => await store.GetAsync(1, "photos/a.jpg"));
        await Assert.ThrowsAnyAsync<IOException>(async () => await store.GetAsync(1, "photos/A.jpg"));

        // A differently-cased prefix is not part of the deleted subtree.
        Assert.Equal("keep"u8.ToArray(), await ReadAllAsync(store.GetAsync(1, "Photos/keep.txt")));
    }

    private object? Scalar(string sql)
    {
        var connectionString = _dsn["postgres:".Length..];
        using var connection = new NpgsqlConnection(connectionString);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        return command.ExecuteScalar();
    }

    private List<string> Query(string sql)
    {
        var connectionString = _dsn["postgres:".Length..];
        using var connection = new NpgsqlConnection(connectionString);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        using var reader = command.ExecuteReader();
        var results = new List<string>();
        while (reader.Read())
        {
            results.Add(reader.GetString(0));
        }

        return results;
    }

    private static async Task<byte[]> ReadAllAsync(Task<Stream> streamTask)
    {
        await using var stream = await streamTask;
        using var buffer = new MemoryStream();
        await stream.CopyToAsync(buffer);
        return buffer.ToArray();
    }
}
