using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.AspNetCore.TestHost;
using Microsoft.Data.Sqlite;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Protocols;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

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

        // rstash is a relying party against itself, so the OpenID Connect handler makes
        // real HTTP calls to its own discovery and token endpoints. There is no socket
        // to reach under TestServer, so point the back channel back at the in-memory
        // server — otherwise every challenge fails with a 500.
        //
        // Setting BackchannelHttpHandler is not enough: registered here, this runs
        // after the handler's built-in post-configure, which has already built both the
        // Backchannel and a ConfigurationManager wrapped around it. Both have to be
        // replaced, and MetadataAddress is readable by now because that same
        // post-configure derived it from Authority.
        builder.ConfigureTestServices(services =>
            services.AddSingleton<IPostConfigureOptions<OpenIdConnectOptions>>(provider =>
                new PostConfigureOptions<OpenIdConnectOptions>(
                    OpenIdConnectDefaults.AuthenticationScheme,
                    options =>
                    {
                        options.Backchannel = new HttpClient(new LoopbackBackchannelHandler(provider));
                        options.ConfigurationManager =
                            new ConfigurationManager<OpenIdConnectConfiguration>(
                                options.MetadataAddress,
                                new OpenIdConnectConfigurationRetriever(),
                                new HttpDocumentRetriever(options.Backchannel) { RequireHttps = false });
                    })));
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
