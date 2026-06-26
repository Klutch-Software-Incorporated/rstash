using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using MudBlazor.Services;
using Rstash.Database;
using Rstash.Notifications;
using Rstash.Server.Components;
using Rstash.Server.Endpoints;
using Rstash.Services;
using Rstash.Services.Storage;
using Rstash.Storage;

// CLI: `rstash env` / `rstash check` short-circuit before the web host starts.
if (args is ["env", ..])
{
    Rstash.Server.Cli.PrintEnvTemplate();
    return;
}

if (args is ["check", ..])
{
    Environment.Exit(await Rstash.Server.Cli.CheckAsync(
        new ConfigurationBuilder().AddEnvironmentVariables().Build()));
}

var builder = WebApplication.CreateBuilder(args);

// Boot-critical configuration (env vars; see the settings registry for the rest).
var databaseDsn = builder.Configuration["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
var blobDsn = builder.Configuration["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";
var baseUrl = (builder.Configuration["RSTASH_BASE_URL"] ?? "http://localhost:8080").TrimEnd('/');

// Core services.
builder.Services.AddHealthChecks();
builder.Services.AddOpenApi();
builder.Services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
builder.Services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
builder.Services.AddSingleton<SettingsService>();
builder.Services.AddSingleton<RemoteStorageService>();
builder.Services.AddSingleton<SetupState>();
builder.Services.AddSingleton<TokenStore>();
builder.Services.AddSingleton<AuditService>();
builder.Services.AddSingleton(EmailSenderFactory.Create(builder.Configuration["RSTASH_EMAIL"]));

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
builder.Services.ConfigureApplicationCookie(options =>
{
    options.LoginPath = "/login";
    options.AccessDeniedPath = "/login";
    options.ReturnUrlParameter = "redirect";
});

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

// Security response headers (CSP tuned for Blazor + MudBlazor + Google Fonts).
const string contentSecurityPolicy =
    "default-src 'self'; " +
    "script-src 'self' 'unsafe-inline'; " +
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
    "img-src 'self' data:; " +
    "font-src 'self' https://fonts.gstatic.com; " +
    "connect-src 'self'; " +
    "frame-ancestors 'none'";
var isHttps = baseUrl.StartsWith("https", StringComparison.OrdinalIgnoreCase);
app.Use(async (context, next) =>
{
    var headers = context.Response.Headers;
    headers["X-Content-Type-Options"] = "nosniff";
    headers["X-Frame-Options"] = "DENY";
    headers["Referrer-Policy"] = "strict-origin-when-cross-origin";
    headers["Content-Security-Policy"] = contentSecurityPolicy;
    if (isHttps)
    {
        headers["Strict-Transport-Security"] = "max-age=63072000; includeSubDomains";
    }

    await next();
});

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
app.MapOpenApi();
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
app.MapOAuthEndpoints();
app.MapFileBrowserEndpoints();
app.MapAdminUserEndpoints();

app.Run();

/// <summary>Exposed for WebApplicationFactory-based integration tests.</summary>
public partial class Program { }
