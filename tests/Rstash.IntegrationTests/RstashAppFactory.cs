using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;

namespace Rstash.IntegrationTests;

/// <summary>
/// Boots the real Server host against throwaway SQLite databases in a temp
/// directory so integration tests exercise the full DI graph + migrations.
/// </summary>
public sealed class RstashAppFactory : WebApplicationFactory<Program>
{
    private readonly string _dir = Path.Combine(Path.GetTempPath(), $"rstash-it-{Guid.NewGuid():N}");
    private readonly IReadOnlyDictionary<string, string> _settings;

    /// <summary>The default host. Must stay the only *public* constructor: xUnit
    /// rejects a class fixture that declares more than one.</summary>
    public RstashAppFactory()
        : this(new Dictionary<string, string>())
    {
    }

    /// <summary>Overrides boot-critical env settings, for tests that need a host
    /// configured differently (e.g. behind a trusted reverse proxy). Internal so it
    /// stays invisible to xUnit's fixture constructor check.</summary>
    internal RstashAppFactory(IReadOnlyDictionary<string, string> settings)
    {
        _settings = settings;
        Directory.CreateDirectory(_dir);
    }

    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseSetting("RSTASH_DB", $"sqlite:{Path.Combine(_dir, "meta.sqlite")}");
        builder.UseSetting("RSTASH_BLOB", $"sqlite:{Path.Combine(_dir, "blobs.sqlite")}");

        foreach (var (key, value) in _settings)
        {
            builder.UseSetting(key, value);
        }
    }

    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (!disposing)
        {
            return;
        }

        SqliteConnection.ClearAllPools();
        try
        {
            Directory.Delete(_dir, recursive: true);
        }
        catch (IOException)
        {
        }
    }
}
