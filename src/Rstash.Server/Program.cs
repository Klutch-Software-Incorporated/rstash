using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using MudBlazor.Services;
using Rstash.Database;
using Rstash.Server.Components;
using Rstash.Server.Endpoints;
using Rstash.Services;
using Rstash.Services.Storage;
using Rstash.Storage;

var builder = WebApplication.CreateBuilder(args);

// Boot-critical configuration (env vars; see the settings registry for the rest).
var databaseDsn = builder.Configuration["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
var blobDsn = builder.Configuration["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";
var baseUrl = (builder.Configuration["RSTASH_BASE_URL"] ?? "http://localhost:8080").TrimEnd('/');

// Core services.
builder.Services.AddHealthChecks();
builder.Services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
builder.Services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
builder.Services.AddSingleton<SettingsService>();
builder.Services.AddSingleton<RemoteStorageService>();
builder.Services.AddSingleton<SetupState>();
builder.Services.AddSingleton<TokenStore>();

// CORS for the storage API (remoteStorage clients run in browsers).
builder.Services.AddCors(options => options.AddPolicy("rstash-storage", policy => policy
    .AllowAnyOrigin()
    .WithMethods("GET", "HEAD", "PUT", "DELETE", "OPTIONS")
    .AllowAnyHeader()
    .WithExposedHeaders("ETag", "Content-Length", "Content-Type")));

// Scoped context bridge: Identity's EF stores need a per-request RstashDbContext,
// while the singleton services above use the factory directly.
builder.Services.AddScoped<RstashDbContext>(sp =>
    sp.GetRequiredService<IDbContextFactory<RstashDbContext>>().CreateDbContext());

// ASP.NET Core Identity (core APIs only — the setup/login UI is custom Blazor).
builder.Services.AddAuthentication(IdentityConstants.ApplicationScheme).AddIdentityCookies();
builder.Services
    .AddIdentityCore<ApplicationUser>(options =>
    {
        options.SignIn.RequireConfirmedAccount = false;
        options.User.RequireUniqueEmail = false;
    })
    .AddRoles<IdentityRole<long>>()
    .AddEntityFrameworkStores<RstashDbContext>()
    .AddSignInManager()
    .AddDefaultTokenProviders();
builder.Services.AddAuthorization();
builder.Services.AddCascadingAuthenticationState();

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
app.UseCors();
app.UseAuthentication();
app.UseAuthorization();
app.UseAntiforgery();

// First-run setup guard: until an account exists, route everything to /setup.
app.Use(async (context, next) =>
{
    var setup = context.RequestServices.GetRequiredService<SetupState>();
    if (!setup.IsComplete)
    {
        var path = context.Request.Path;
        var exempt = path.StartsWithSegments("/setup")
            || path.StartsWithSegments("/healthz")
            || path.StartsWithSegments("/storage")
            || path.StartsWithSegments("/.well-known")
            || path.StartsWithSegments("/oauth")
            || path.StartsWithSegments("/_")
            || (path.Value?.Contains('.') ?? false);

        var contextFactory = context.RequestServices.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();
        if (await db.Users.AnyAsync())
        {
            setup.MarkComplete();
        }
        else if (!exempt)
        {
            context.Response.Redirect("/setup");
            return;
        }
    }

    await next();
});

app.MapHealthChecks("/healthz");
app.MapRazorComponents<App>()
    .AddInteractiveServerRenderMode()
    .AddAdditionalAssemblies(typeof(Rstash.Web.Layout.MainLayout).Assembly);

app.MapPost("/auth/logout", async (SignInManager<ApplicationUser> signInManager) =>
{
    await signInManager.SignOutAsync();
    return Results.Redirect("/");
}).DisableAntiforgery();

// remoteStorage storage API (bearer-token auth + scopes).
app.MapStorageEndpoints();
app.MapWebFinger(baseUrl);
// OAuth authorize/token/revoke land next in P4.

app.Run();

/// <summary>Exposed for WebApplicationFactory-based integration tests.</summary>
public partial class Program { }
