using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>
/// Covers shareable in-memory SQLite (<c>sqlite::memory:</c>) — the local-dev/test
/// convenience that starts from a clean database on every process start. The property
/// that matters (and that a naive <c>:memory:</c> fails) is that the schema and data
/// survive across the separate connections the migrator and EF each open.
/// </summary>
public sealed class SqliteInMemoryTests
{
    // Distinct name per test so the shared-cache databases don't bleed into each other.
    private static string MemoryDsn() => $"sqlite::memory:{Guid.NewGuid():N}";

    [Fact]
    public void ToSqliteConnectionString_MapsMemorySentinel_ToSharedCache()
    {
        var connectionString = RstashDbContextOptionsExtensions.ToSqliteConnectionString(":memory:foo");
        var builder = new SqliteConnectionStringBuilder(connectionString);

        Assert.Equal("foo", builder.DataSource);
        Assert.Equal(SqliteOpenMode.Memory, builder.Mode);
        Assert.Equal(SqliteCacheMode.Shared, builder.Cache);
    }

    [Fact]
    public async Task InMemoryDatabase_RetainsSchemaAndData_AcrossSeparateContexts()
    {
        var dsn = MemoryDsn();

        // The migrator opens its own connection, creates the schema, then closes it.
        // Without the keep-alive this is exactly where a plain :memory: database dies.
        SchemaMigrator.MigrateUp(dsn);

        var options = new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options;

        await using (var write = new RstashDbContext(options))
        {
            write.Settings.Add(new Setting
            {
                Key = "registration_mode",
                Value = "open",
                UpdatedAt = DateTimeOffset.UnixEpoch,
            });
            await write.SaveChangesAsync();
        }

        // A brand-new context (new connection) must still see the migrated schema and the row.
        await using (var read = new RstashDbContext(options))
        {
            Assert.Equal("open", (await read.Settings.SingleAsync()).Value);
        }
    }

    [Fact]
    public async Task NamedInMemoryDatabases_AreIsolatedFromEachOther()
    {
        var first = MemoryDsn();
        var second = MemoryDsn();

        SchemaMigrator.MigrateUp(first);
        SchemaMigrator.MigrateUp(second);

        await using var firstCtx = new RstashDbContext(
            new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(first).Options);
        firstCtx.Settings.Add(new Setting { Key = "k", Value = "v", UpdatedAt = DateTimeOffset.UnixEpoch });
        await firstCtx.SaveChangesAsync();

        await using var secondCtx = new RstashDbContext(
            new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(second).Options);

        Assert.Equal(0, await secondCtx.Settings.CountAsync()); // the write above landed only in `first`
    }

    [Theory]
    [InlineData("sqlite::memory:", "sqlite::memory:")]                 // both bare → same default database
    [InlineData("sqlite::memory:shared", "sqlite::memory:shared")]     // same explicit name
    public void EnsureDistinctInMemoryDatabases_Throws_WhenBothNameTheSameDatabase(string metaDsn, string blobDsn)
    {
        var ex = Assert.Throws<InvalidOperationException>(() =>
            RstashDbContextOptionsExtensions.EnsureDistinctInMemoryDatabases(metaDsn, blobDsn));

        Assert.Contains("distinct", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("sqlite::memory:", "sqlite::memory:blobs")] // distinct in-memory names
    [InlineData("sqlite::memory:", "sqlite:blobs.sqlite")]  // blob is a file
    [InlineData("sqlite:meta.sqlite", "sqlite:meta.sqlite")] // both files (may share safely)
    [InlineData("sqlite::memory:", "azureblob://acct/container")] // blob is a remote backend
    public void EnsureDistinctInMemoryDatabases_Allows_WhenNotTheSameInMemoryDatabase(string metaDsn, string blobDsn)
    {
        RstashDbContextOptionsExtensions.EnsureDistinctInMemoryDatabases(metaDsn, blobDsn); // does not throw
    }
}
