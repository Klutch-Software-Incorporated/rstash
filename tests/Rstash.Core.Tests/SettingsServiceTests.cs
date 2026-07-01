using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

/// <summary>End-to-end settings service against a real SQLite database.</summary>
public class SettingsServiceTests
{
    [Fact]
    public async Task Set_Delete_Reload_RoundtripsThroughDatabase()
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-settings-{Guid.NewGuid():N}.sqlite");
        IDbContextFactory<RstashDbContext> factory = new SqliteContextFactory($"sqlite:{path}");

        try
        {
            SchemaMigrator.MigrateUp($"sqlite:{path}");

            var settings = new SettingsService(factory);
            await settings.ReloadAsync();
            Assert.Equal("closed", settings.Current.RegistrationMode);

            await settings.SetAsync("registration_mode", "open");
            Assert.Equal("open", settings.Current.RegistrationMode);

            await settings.SetAsync("max_upload_size", "100MB");
            Assert.Equal(100L * 1024 * 1024, settings.Current.MaxUploadSize);

            // Invalid writes are rejected and leave the snapshot unchanged.
            await Assert.ThrowsAsync<SettingValidationException>(
                () => settings.SetAsync("registration_mode", "bogus"));
            Assert.Equal("open", settings.Current.RegistrationMode);

            // Deleting an override reverts to the default.
            await settings.DeleteAsync("registration_mode");
            Assert.Equal("closed", settings.Current.RegistrationMode);

            Assert.Equal("100MB", await settings.GetOverrideAsync("max_upload_size"));
        }
        finally
        {
            SqliteConnection.ClearAllPools();
            foreach (var p in new[] { path, path + "-wal", path + "-shm" })
            {
                try
                {
                    if (File.Exists(p))
                    {
                        File.Delete(p);
                    }
                }
                catch (IOException)
                {
                }
            }
        }
    }

    private sealed class SqliteContextFactory(string dsn) : IDbContextFactory<RstashDbContext>
    {
        public RstashDbContext CreateDbContext() =>
            new(new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options);
    }
}
