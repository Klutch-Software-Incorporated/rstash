using Microsoft.AspNetCore.DataProtection;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// Guards the fix for a live bug: with the default (filesystem) key provider, every
/// process restart generated a fresh Data Protection key ring, silently invalidating
/// every auth cookie and antiforgery token. Persisting the ring to the database is
/// what makes sessions survive a restart — and what lets a multi-instance deployment
/// share one ring instead of each instance rejecting the others' cookies.
/// </summary>
public sealed class DataProtectionPersistenceTests : IDisposable
{
    private readonly string _path =
        Path.Combine(Path.GetTempPath(), $"rstash-dpapi-{Guid.NewGuid():N}.sqlite");

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
    public async Task ProtectedPayload_SurvivesA_ProcessRestart()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        string ciphertext;
        await using (var first = BuildHost())
        {
            ciphertext = Protector(first).Protect("session-for-alice");
        }

        // A second, wholly independent host over the same database — the same thing a
        // container restart does.
        //
        // Note this assertion alone does NOT prove the fix: strip
        // PersistKeysToDbContext and it still passes, because Data Protection then
        // falls back to a shared filesystem location that both hosts read. That
        // fallback is also why the bug never reproduced on a dev box and only bit the
        // container deploy, whose filesystem is ephemeral. The companion test below is
        // what pins the ring to the database; this one proves the DB-backed ring
        // actually round-trips, which a row-count assertion cannot show.
        await using (var second = BuildHost())
        {
            Assert.Equal("session-for-alice", Protector(second).Unprotect(ciphertext));
        }
    }

    [Fact]
    public async Task ProtectingA_Payload_WritesTheKeyRingToTheDatabase()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        await using (var host = BuildHost())
        {
            Protector(host).Protect("anything");
        }

        // Proves the ring landed in our table rather than falling back to the
        // filesystem/ephemeral providers, which would still let the test above pass
        // within a single process on some machines.
        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase($"sqlite:{_path}")
            .Options;
        await using var ctx = new RstashDbContext(options);
        Assert.NotEmpty(await ctx.DataProtectionKeys.ToListAsync());
    }

    private static IDataProtector Protector(IServiceProvider provider) =>
        provider.GetRequiredService<IDataProtectionProvider>().CreateProtector("rstash.tests");

    /// <summary>Mirrors the host's registration in Program.cs.</summary>
    private ServiceProvider BuildHost()
    {
        var services = new ServiceCollection();
        services.AddDbContext<RstashDbContext>(options => options.UseRstashDatabase($"sqlite:{_path}"));
        services.AddDataProtection()
            .SetApplicationName("rstash")
            .PersistKeysToDbContext<RstashDbContext>();
        return services.BuildServiceProvider();
    }
}
