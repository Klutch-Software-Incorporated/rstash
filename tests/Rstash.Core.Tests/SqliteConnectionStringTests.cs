using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// Guards SQLite DSN handling. The regression that prompted these: a production
/// DSN of the form <c>sqlite:/home/rstash.sqlite?journal_mode=delete</c> crashed
/// boot because the old heuristic (any '=' means "already a connection string")
/// fed the whole path+query to <see cref="SqliteConnectionStringBuilder"/>, which
/// rejected <c>/home/rstash.sqlite?journal_mode</c> as an unknown keyword.
///
/// <c>journal_mode</c> is environment-critical: the hosted deployment keeps its
/// SQLite files on an Azure Files (SMB) share and must use <c>delete</c> — WAL
/// corrupts the database over SMB — so the app must honor the DSN's mode and must
/// never force WAL.
/// </summary>
public class SqliteConnectionStringTests
{
    [Theory]
    [InlineData("rstash.sqlite", "rstash.sqlite")]
    [InlineData("/home/rstash.sqlite", "/home/rstash.sqlite")]
    [InlineData("/home/rstash.sqlite?journal_mode=delete", "/home/rstash.sqlite")]     // query stripped
    [InlineData("/home/rstash.sqlite?journal_mode=delete&cache=shared", "/home/rstash.sqlite")]
    [InlineData(@"C:\data\rstash.sqlite", @"C:\data\rstash.sqlite")]                    // Windows path, no '=' misfire
    public void ToSqliteConnectionString_BuildsDataSource_StrippingAnyQuery(string dsnRemainder, string expectedDataSource)
    {
        var connectionString = RstashDbContextOptionsExtensions.ToSqliteConnectionString(dsnRemainder);

        Assert.Equal(expectedDataSource, new SqliteConnectionStringBuilder(connectionString).DataSource);
    }

    [Fact]
    public void ToSqliteConnectionString_PassesThroughAnExistingConnectionString()
    {
        const string connectionString = "Data Source=x.db;Mode=ReadOnly";

        Assert.Equal(connectionString, RstashDbContextOptionsExtensions.ToSqliteConnectionString(connectionString));
    }

    [Theory]
    [InlineData("/home/rstash.sqlite?journal_mode=delete", "delete")]
    [InlineData("/home/rstash.sqlite?journal_mode=WAL", "WAL")]
    [InlineData("/home/rstash.sqlite?foo=bar&journal_mode=delete", "delete")]
    [InlineData("/home/rstash.sqlite", null)]                                          // no query
    [InlineData("rstash.sqlite?cache=shared", null)]                                   // query without journal_mode
    public void SqliteJournalMode_ReadsTheQueryParameter(string dsnRemainder, string? expected)
    {
        Assert.Equal(expected, RstashDbContextOptionsExtensions.SqliteJournalMode(dsnRemainder));
    }

    [Fact]
    public void UseRstashDatabase_InvalidJournalMode_ThrowsClearError()
    {
        var ex = Assert.Throws<ArgumentException>(() =>
            new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase("sqlite:x.db?journal_mode=bogus"));

        Assert.Contains("journal_mode", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("delete")] // the production (Azure Files / SMB) mode — must NOT become WAL
    [InlineData("wal")]    // honored the other way too, proving it's DSN-driven not hard-coded
    public async Task ProductionDsnForm_HonorsJournalModeFromDsn(string journalMode)
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-dsn-{Guid.NewGuid():N}.sqlite");
        var dsn = $"sqlite:{path}?journal_mode={journalMode}"; // the exact shape that crashed boot

        try
        {
            // Both the migrator and EF must tolerate the query-string DSN.
            SchemaMigrator.MigrateUp(dsn);

            var options = new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options;
            await using (var ctx = new RstashDbContext(options))
            {
                // A real query opens the connection, running
                // SqliteConnectionInitInterceptor, which applies PRAGMA
                // journal_mode with the DSN's value.
                Assert.Equal(0, await ctx.Settings.CountAsync());
            }

            using var connection = new SqliteConnection($"Data Source={path}");
            connection.Open();
            using var command = connection.CreateCommand();
            command.CommandText = "PRAGMA journal_mode;";
            Assert.Equal(journalMode, ((string)command.ExecuteScalar()!).ToLowerInvariant());
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
