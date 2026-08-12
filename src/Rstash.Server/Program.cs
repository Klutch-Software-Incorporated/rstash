using Microsoft.AspNetCore.DataProtection;
using Microsoft.AspNetCore.HttpOverrides;
using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using MudBlazor.Services;
using Rstash.Database;
using Rstash.Notifications;
using Rstash.Server.Components;
using Rstash.Server.Endpoints;
using Rstash.Server.Health;
using Rstash.Services;
using Rstash.Services.Configuration;
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

if (args is ["seed", ..])
{
    Environment.Exit(await Rstash.Server.Cli.SeedAsync(
        new ConfigurationBuilder().AddEnvironmentVariables().Build(),
        args.Length > 1 ? args[1] : null));
}

var builder = WebApplication.CreateBuilder(args);

// Serve RCL static assets (MudBlazor CSS/JS at _content/…) in every environment, not just
// Development. Reads the build-output manifest if present; no-op for published output, which
// carries its own embedded endpoint manifest. Without this a non-published Production run
// (e.g. launching the built DLL directly) serves the UI completely unstyled.
builder.WebHost.UseStaticWebAssets();

// Boot-critical configuration (env vars; see the settings registry for the rest).
var databaseDsn = builder.Configuration["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
var blobDsn = builder.Configuration["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";
// Every absolute URL rstash emits derives from this, never from the incoming request
// (see BaseUrl). A malformed value breaks WebFinger and OAuth redirects, so fail at
// boot rather than serving links nobody can follow.
var baseUrl = BaseUrl.ResolveOrThrow(builder.Configuration[EnvVars.BaseUrl]);

// Reverse-proxy support is opt-in. X-Forwarded-* headers are trivially spoofed by
// anyone who can reach the server, and ASP.NET Core's default trust list (loopback
// only) silently ignores them in a container anyway — so rather than half-work, honour
// them only when the operator confirms a proxy is in front.
var trustProxy = builder.Configuration.GetValue(EnvVars.TrustProxy, false);

// In-memory SQLite is a per-process dev/test convenience; the metadata and blob DSNs
// must name DISTINCT in-memory databases (one can't hold both schemas).
RstashDbContextOptionsExtensions.EnsureDistinctInMemoryDatabases(databaseDsn, blobDsn);

// Listen on RSTASH_ADDR (host:port; empty host = all interfaces), default :8080.
var listenAddr = builder.Configuration["RSTASH_ADDR"] ?? ":8080";
builder.WebHost.UseUrls(listenAddr.StartsWith(':') ? $"http://0.0.0.0{listenAddr}" : $"http://{listenAddr}");

// Core services.
// Connectivity checks are bounded so /healthz fails fast instead of blocking on
// SDK retries when a dependency is down (load balancers poll this).
builder.Services.AddHealthChecks()
    .AddCheck<DatabaseHealthCheck>(
        "database",
        failureStatus: Microsoft.Extensions.Diagnostics.HealthChecks.HealthStatus.Unhealthy,
        tags: null,
        timeout: TimeSpan.FromSeconds(5))
    .AddCheck<StorageHealthCheck>(
        "storage",
        failureStatus: Microsoft.Extensions.Diagnostics.HealthChecks.HealthStatus.Unhealthy,
        tags: null,
        timeout: TimeSpan.FromSeconds(5));
builder.Services.AddOpenApi();
builder.Services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
builder.Services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
builder.Services.AddSingleton<SettingsService>();

builder.Services.AddSingleton<RemoteStorageService>();
builder.Services.AddSingleton<EgressTracker>();
builder.Services.AddHostedService(sp => sp.GetRequiredService<EgressTracker>());
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

// Loads the current user once per request/circuit so the layout and page don't issue
// concurrent UserManager reads on the shared scoped RstashDbContext (see CurrentUserAccessor).
builder.Services.AddScoped<Rstash.Web.CurrentUserAccessor>();

// Data Protection keys encrypt the auth cookie and antiforgery tokens. The default
// provider stores them in the container filesystem, so every restart wipes the ring
// and signs out every user; persisting to the database fixes that and lets a
// multi-instance deployment share one ring. The application name isolates the ring,
// so changing it invalidates every existing cookie.
builder.Services.AddDataProtection()
    .SetApplicationName("rstash")
    .PersistKeysToDbContext<RstashDbContext>();

// Humans sign in with a username and password against ASP.NET Core Identity, and the
// resulting cookie authorizes every subsequent request. This is deliberately separate
// from the remoteStorage app-authorization server at /oauth/*, which hands bearer
// tokens to third-party apps rather than signing in people.
builder.Services.AddAuthentication(IdentityConstants.ApplicationScheme)
    .AddIdentityCookies();

builder.Services.ConfigureApplicationCookie(options =>
{
    options.Cookie.Name = "rstash.session";
    options.Cookie.HttpOnly = true;
    options.Cookie.SameSite = SameSiteMode.Lax;
    options.SlidingExpiration = true;
    options.LoginPath = "/login";
    options.AccessDeniedPath = "/login";
    options.ReturnUrlParameter = "redirect";
});

// ASP.NET Core Identity (core APIs only — the setup/login UI is custom Blazor).
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

// Apply the schema (FluentMigrator owns DDL) and load runtime settings before serving.
SchemaMigrator.MigrateUp(databaseDsn);
await using (var scope = app.Services.CreateAsyncScope())
{
    await scope.ServiceProvider.GetRequiredService<SettingsService>().ReloadAsync();
}

// Must precede any middleware that reads the scheme, host, or client IP. Clearing the
// known-network/proxy lists is what makes this work in a container, where the proxy's
// address is not knowable ahead of time; it is safe only because reaching this line at
// all required the operator to opt in via RSTASH_TRUST_PROXY.
if (trustProxy)
{
    app.UseForwardedHeaders(new ForwardedHeadersOptions
    {
        ForwardedHeaders = ForwardedHeaders.XForwardedProto
            | ForwardedHeaders.XForwardedHost
            | ForwardedHeaders.XForwardedFor,
        KnownNetworks = { },
        KnownProxies = { },
    });
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

    await next(context);
});

app.MapStaticAssets();
app.UseCors();

// First-run setup guard: until an account exists, route everything to /setup.
//
// Ahead of authorization deliberately. Pages carry [Authorize], and authorization
// challenges before this ever runs if it sits behind it — so a brand-new server would
// send its very first visitor to a login form to prove an identity that cannot exist
// yet, instead of to the wizard that creates it. This only reads the request path and
// the user table, so it has no business running after authentication anyway.
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
            || path.StartsWithSegments("/ui")
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

    await next(context);
});

app.UseAuthentication();
app.UseAuthorization();
app.UseAntiforgery();

app.MapHealthChecks("/healthz", new Microsoft.AspNetCore.Diagnostics.HealthChecks.HealthCheckOptions
{
    ResponseWriter = HealthResponse.WriteAsync,
});
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
app.MapAccountEndpoints();
app.MapUiEndpoints();

app.Run();

/// <summary>Exposed for WebApplicationFactory-based integration tests.</summary>
public partial class Program { }
