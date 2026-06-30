using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Core.Tests;

public sealed class SqliteJournalModeTests : IDisposable
{
    private readonly List<string> _paths = [];

    [Fact]
    public void JournalMode_DsnParam_DeleteIsApplied()
    {
        Assert.Equal("delete", QueryJournalMode($"sqlite:{TempDb()}?journal_mode=delete"));
    }

    [Fact]
    public void JournalMode_DsnParam_WalIsApplied()
    {
        // Proves the param actually drives the mode (not just coincidence with a default).
        Assert.Equal("wal", QueryJournalMode($"sqlite:{TempDb()}?journal_mode=wal"));
    }

    [Fact]
    public void JournalMode_InvalidValue_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase($"sqlite:{TempDb()}?journal_mode=bogus"));
    }

    private string TempDb()
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-jm-{Guid.NewGuid():N}.db");
        _paths.Add(path);
        return path;
    }

    private static string QueryJournalMode(string dsn)
    {
        var options = new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options;
        using var context = new RstashDbContext(options);
        context.Database.OpenConnection();
        using var command = context.Database.GetDbConnection().CreateCommand();
        command.CommandText = "PRAGMA journal_mode;";
        return ((string)command.ExecuteScalar()!).ToLowerInvariant();
    }

    public void Dispose()
    {
        foreach (var path in _paths)
        {
            foreach (var suffix in new[] { "", "-wal", "-shm", "-journal" })
            {
                try { File.Delete(path + suffix); } catch { /* best effort */ }
            }
        }
    }
}
