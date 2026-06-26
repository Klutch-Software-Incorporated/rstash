using Microsoft.EntityFrameworkCore;
using MudBlazor.Services;
using Rstash.Database;
using Rstash.Server.Components;
using Rstash.Services;
using Rstash.Services.Storage;
using Rstash.Storage;

var builder = WebApplication.CreateBuilder(args);

// Boot-critical configuration (env vars; see the settings registry for the rest).
var databaseDsn = builder.Configuration["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
var blobDsn = builder.Configuration["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";

// Core services.
builder.Services.AddHealthChecks();
builder.Services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
builder.Services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
builder.Services.AddSingleton<SettingsService>();
builder.Services.AddSingleton<RemoteStorageService>();

// Blazor Web App (interactive server) + MudBlazor.
builder.Services.AddRazorComponents().AddInteractiveServerComponents();
builder.Services.AddMudServices();

var app = builder.Build();

// Apply pending migrations and load runtime settings before serving.
await using (var scope = app.Services.CreateAsyncScope())
{
    var contextFactory = scope.ServiceProvider.GetRequiredService<IDbContextFactory<RstashDbContext>>();
    await using var db = await contextFactory.CreateDbContextAsync();
    await db.Database.MigrateAsync();

    await scope.ServiceProvider.GetRequiredService<SettingsService>().ReloadAsync();
}

app.MapStaticAssets();
app.UseAntiforgery();

app.MapHealthChecks("/healthz");
app.MapRazorComponents<App>()
    .AddInteractiveServerRenderMode()
    .AddAdditionalAssemblies(typeof(Rstash.Web.Layout.MainLayout).Assembly);

// Storage protocol endpoints (GET/PUT/DELETE/HEAD /storage/...), WebFinger, and
// OAuth land in P4 — they need bearer-token auth. Identity + setup/login UI are
// built across the rest of P3.

app.Run();

/// <summary>Exposed for WebApplicationFactory-based integration tests.</summary>
public partial class Program { }
