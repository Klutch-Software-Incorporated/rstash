using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// Guards the SQLite DSN → connection-string translation. The regression that
/// prompted these: a production DSN of the form
/// <c>sqlite:/home/rstash.sqlite?journal_mode=WAL</c> crashed boot because the
/// old heuristic (any '=' means "already a connection string") fed the whole
/// path+query to <see cref="SqliteConnectionStringBuilder"/>, which rejected
/// <c>/home/rstash.sqlite?journal_mode</c> as an unknown keyword.
/// </summary>
public class SqliteConnectionStringTests
{
    [Theory]
    [InlineData("rstash.sqlite", "rstash.sqlite")]
    [InlineData(":memory:", ":memory:")]
    [InlineData("/home/rstash.sqlite", "/home/rstash.sqlite")]
    [InlineData("/home/rstash.sqlite?journal_mode=WAL", "/home/rstash.sqlite")]       // query stripped
    [InlineData("/home/rstash.sqlite?journal_mode=WAL&cache=shared", "/home/rstash.sqlite")]
    [InlineData(@"C:\data\rstash.sqlite", @"C:\data\rstash.sqlite")]                  // Windows path, no '=' misfire
    public void BuildsDataSource_StrippingAnyQuery(string dsnRemainder, string expectedDataSource)
    {
        var connectionString = RstashDbContextOptionsExtensions.ToSqliteConnectionString(dsnRemainder);

        Assert.Equal(expectedDataSource, new SqliteConnectionStringBuilder(connectionString).DataSource);
    }

    [Fact]
    public void PassesThroughAnExistingConnectionString()
    {
        const string connectionString = "Data Source=x.db;Mode=ReadOnly";

        Assert.Equal(connectionString, RstashDbContextOptionsExtensions.ToSqliteConnectionString(connectionString));
    }

    [Fact]
    public async Task ProductionDsnForm_WithQueryParams_MigratesAndEnablesWal()
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-dsn-{Guid.NewGuid():N}.sqlite");
        var dsn = $"sqlite:{path}?journal_mode=WAL"; // the exact shape that crashed boot

        try
        {
            // Both the migrator and EF must tolerate the query-string DSN.
            SchemaMigrator.MigrateUp(dsn);

            var options = new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options;
            await using (var ctx = new RstashDbContext(options))
            {
                // A real query opens the connection, running
                // SqliteConnectionInitInterceptor, which applies PRAGMA
                // journal_mode=WAL (persisted to the database file).
                Assert.Equal(0, await ctx.Settings.CountAsync());
            }

            using var connection = new SqliteConnection($"Data Source={path}");
            connection.Open();
            using var command = connection.CreateCommand();
            command.CommandText = "PRAGMA journal_mode;";
            Assert.Equal("wal", ((string)command.ExecuteScalar()!).ToLowerInvariant());
        }
        finally
        {
            SqliteConnection.ClearAllPools();
            foreach (var p in new[] { path, path + "-wal", path + "-shm" })
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
    }
}
