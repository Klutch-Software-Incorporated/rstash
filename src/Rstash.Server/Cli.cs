using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services.Configuration;
using Rstash.Storage;

namespace Rstash.Server;

/// <summary>The minimal CLI: <c>env</c> (print config template) and <c>check</c>
/// (validate config + connectivity). Everything else runs the server.</summary>
internal static class Cli
{
    public static void PrintEnvTemplate()
    {
        Console.WriteLine("# rstash configuration (environment variables).");
        Console.WriteLine("# Only boot-critical settings use env vars; everything else is managed");
        Console.WriteLine("# at runtime in the admin UI.");
        Console.WriteLine();

        foreach (var def in SettingDefinitions.All.Where(d => d.EnvVar is not null))
        {
            Console.WriteLine($"# {def.Description}");
            Console.WriteLine($"{def.EnvVar}={def.Default}");
            Console.WriteLine();
        }
    }

    public static async Task<int> CheckAsync(IConfiguration config)
    {
        var databaseDsn = config["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
        var blobDsn = config["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";
        var ok = true;

        try
        {
            await using var db = new RstashDbContext(
                new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(databaseDsn).Options);
            await db.Database.CanConnectAsync();
            Console.WriteLine($"[ok]   database:   {databaseDsn}");
        }
        catch (Exception ex)
        {
            ok = false;
            Console.WriteLine($"[FAIL] database:   {databaseDsn} — {ex.Message}");
        }

        try
        {
            await using var store = StorageFactory.Open(blobDsn);
            Console.WriteLine($"[ok]   blob store: {blobDsn}");
        }
        catch (Exception ex)
        {
            ok = false;
            Console.WriteLine($"[FAIL] blob store: {blobDsn} — {ex.Message}");
        }

        Console.WriteLine(ok ? "\nConfiguration OK." : "\nConfiguration has errors.");
        return ok ? 0 : 1;
    }
}
