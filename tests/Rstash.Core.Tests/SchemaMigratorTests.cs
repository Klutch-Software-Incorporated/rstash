using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>
/// Proves the FluentMigrator schema (<see cref="Rstash.Database.Migrations.InitialCreate"/>)
/// builds cleanly on SQLite and that the EF Core model round-trips against it —
/// the guard that keeps the two schema representations from drifting now that
/// FluentMigrator owns DDL and EF only queries.
/// </summary>
public sealed class SchemaMigratorTests : IDisposable
{
    private readonly string _path = Path.Combine(Path.GetTempPath(), $"rstash-migrate-{Guid.NewGuid():N}.sqlite");

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
    public void MigrateUp_IsIdempotent_AndRecordsVersion()
    {
        // Running twice must be a no-op the second time (VersionInfo tracks applied
        // migrations), not an error from re-creating existing tables.
        SchemaMigrator.MigrateUp($"sqlite:{_path}");
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        using var version = connection.CreateCommand();
        version.CommandText =
            "SELECT COUNT(*) FROM VersionInfo WHERE Version IN (202607010001, 202607280001);";
        Assert.Equal(2L, (long)version.ExecuteScalar()!);
    }

    [Fact]
    public void MigratedSchema_HasAllTablesAndUniqueIndexes()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        var tables = Query(connection, "SELECT name FROM sqlite_master WHERE type = 'table';");
        string[] expectedTables =
        [
            "nodes", "settings", "oauth_tokens", "authorization_codes", "egress_usage", "audit_log",
            "AspNetUsers", "AspNetRoles", "AspNetUserRoles", "AspNetUserClaims",
            "AspNetUserLogins", "AspNetRoleClaims", "AspNetUserTokens",
            "DataProtectionKeys",
        ];
        foreach (var table in expectedTables)
        {
            Assert.Contains(table, tables);
        }

        // The unique indexes are the constraints functional tests may not trip; assert
        // them explicitly. Their names and uniqueness must match the EF-generated schema.
        var uniqueIndexes = Query(
            connection,
            "SELECT name FROM sqlite_master WHERE type = 'index' AND sql LIKE '%UNIQUE%';");
        foreach (var index in new[] { "IX_nodes_UserId_Path", "IX_egress_usage_UserId_Period", "UserNameIndex", "RoleNameIndex" })
        {
            Assert.Contains(index, uniqueIndexes);
        }
    }

    private static List<string> Query(SqliteConnection connection, string sql)
    {
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

    [Fact]
    public async Task EfModel_RoundTripsAgainstMigratedSchema()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase($"sqlite:{_path}")
            .Options;

        await using (var ctx = new RstashDbContext(options))
        {
            // An Identity user (AspNetUsers) plus one row in each of the app tables
            // exercised by the stores — covers the columns/indexes most likely to
            // drift, including the two DEFAULT '' columns left unset below.
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

            ctx.Settings.Add(new Setting { Key = "registration_mode", Value = "open", UpdatedAt = DateTimeOffset.UnixEpoch });

            // The Data Protection key ring: written by the DP stack at runtime, but the
            // schema must match its entity or auth cookies break on first request.
            ctx.DataProtectionKeys.Add(new Microsoft.AspNetCore.DataProtection.EntityFrameworkCore.DataProtectionKey
            {
                FriendlyName = "key-2026-07-28",
                Xml = "<key id=\"test\" />",
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

            var audit = await ctx.AuditLog.SingleAsync();
            Assert.Equal("", audit.Details); // DEFAULT '' applied by the migration.

            Assert.Equal("alice", (await ctx.Users.SingleAsync()).UserName);
            Assert.Equal("open", (await ctx.Settings.SingleAsync()).Value);

            var key = await ctx.DataProtectionKeys.SingleAsync();
            Assert.Equal("key-2026-07-28", key.FriendlyName);
            Assert.Equal("<key id=\"test\" />", key.Xml);
        }
    }
}
