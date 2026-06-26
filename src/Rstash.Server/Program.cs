using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Storage;
using Rstash.Storage;

var builder = WebApplication.CreateBuilder(args);

// Boot-critical configuration (env vars; see the settings registry for the rest).
var databaseDsn = builder.Configuration["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
var blobDsn = builder.Configuration["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";

builder.Services.AddHealthChecks();
builder.Services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
builder.Services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
builder.Services.AddSingleton<SettingsService>();
builder.Services.AddSingleton<RemoteStorageService>();

var app = builder.Build();

// Apply pending migrations and load runtime settings before serving.
await using (var scope = app.Services.CreateAsyncScope())
{
    var contextFactory = scope.ServiceProvider.GetRequiredService<IDbContextFactory<RstashDbContext>>();
    await using var db = await contextFactory.CreateDbContextAsync();
    await db.Database.MigrateAsync();

    await scope.ServiceProvider.GetRequiredService<SettingsService>().ReloadAsync();
}

app.MapHealthChecks("/healthz");

// Storage protocol endpoints (GET/PUT/DELETE/HEAD /storage/...), the Blazor +
// MudBlazor UI, WebFinger, and OAuth land in P3/P4 — they need user identity
// and bearer-token auth, which are built next.

app.Run();

/// <summary>Exposed for WebApplicationFactory-based integration tests.</summary>
public partial class Program { }
