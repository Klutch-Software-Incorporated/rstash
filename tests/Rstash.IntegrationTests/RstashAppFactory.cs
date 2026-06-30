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

    public RstashAppFactory() => Directory.CreateDirectory(_dir);

    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseSetting("RSTASH_DB", $"sqlite:{Path.Combine(_dir, "meta.sqlite")}");
        builder.UseSetting("RSTASH_BLOB", $"sqlite:{Path.Combine(_dir, "blobs.sqlite")}");
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
