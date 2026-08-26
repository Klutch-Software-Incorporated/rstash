using System.Diagnostics.CodeAnalysis;
using System.Net.Security;
using System.Reflection;
using System.Text;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Configuration;
using Rstash.Services.Storage;
using Rstash.Storage;

namespace Rstash.Server;

/// <summary>The minimal CLI: <c>version</c>, <c>env</c> (print config template),
/// <c>check</c> (validate config + connectivity), <c>healthcheck</c> (probe a running
/// server), and <c>seed</c> (populate an account with sample data). Everything else
/// runs the server.</summary>
internal static class Cli
{
    /// <summary>
    /// The build identity: "v0.5.0" from a release, "v0.5.0+dev" from anything else.
    /// Logged at startup and printed by <c>rstash version</c>, so however the server is
    /// running there is a way to ask what it is.
    /// </summary>
    public static string Version =>
        Assembly.GetEntryAssembly()?
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()?
            .InformationalVersion
        ?? "unknown";

    public static void PrintVersion() => Console.WriteLine($"rstash {Version}");

    /// <summary>
    /// Probes <c>/healthz</c> on the local server and exits non-zero if it is not healthy.
    /// This exists so the container image can declare a HEALTHCHECK: the .NET runtime images
    /// ship no curl or wget, and adding one to every deployment for the sake of a health
    /// probe is a poor trade. Talks to the loopback address on the configured port rather
    /// than RSTASH_BASE_URL, which names the *public* URL and may resolve to a proxy.
    /// </summary>
    public static async Task<int> HealthcheckAsync(IConfiguration config)
    {
        var scheme = TlsOptions.TryResolve(
            config[EnvVars.TlsMode],
            config[EnvVars.TlsCert],
            config[EnvVars.TlsKey],
            out var tls,
            out _) && tls.Enabled
            ? "https"
            : "http";

        var url = $"{scheme}://127.0.0.1:{ListenPort(config[EnvVars.Addr])}/healthz";

        using var handler = new HttpClientHandler();
        if (scheme == "https")
        {
            // The certificate is almost never valid for 127.0.0.1, and checking it here would
            // test the wrong thing: this asks "is my own process serving?", not "does a client
            // trust it?" — over loopback, inside the container, with no network in between.
            [SuppressMessage("Security", "CA5359:Do not disable certificate validation",
                Justification = "Loopback liveness probe of our own process; identity is established by the address, not the certificate.")]
            static bool AcceptLoopbackCertificate(
                HttpRequestMessage request, System.Security.Cryptography.X509Certificates.X509Certificate2? certificate,
                System.Security.Cryptography.X509Certificates.X509Chain? chain, SslPolicyErrors errors) => true;

            handler.ServerCertificateCustomValidationCallback = AcceptLoopbackCertificate;
        }

        // Short: Docker's default healthcheck timeout is 30s, and a probe that hangs is a
        // probe that reports nothing.
        using var client = new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(5) };

        try
        {
            using var response = await client.GetAsync(url);
            if (response.IsSuccessStatusCode)
            {
                return 0;
            }

            // /healthz answers 503 with a JSON body naming the failing dependency; surface it,
            // since `docker inspect` keeps the last few probe outputs and that is often the
            // only diagnostic an operator has.
            Console.Error.WriteLine($"unhealthy: {url} returned {(int)response.StatusCode} — {await response.Content.ReadAsStringAsync()}");
            return 1;
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException)
        {
            Console.Error.WriteLine($"unhealthy: {url} — {ex.Message}");
            return 1;
        }
    }

    /// <summary>
    /// Extracts the port from an RSTASH_ADDR value (":8080", "0.0.0.0:8080", "host:8080").
    /// </summary>
    private static string ListenPort(string? addr)
    {
        const string defaultPort = "8080";
        if (string.IsNullOrWhiteSpace(addr))
        {
            return defaultPort;
        }

        var separator = addr.LastIndexOf(':');
        if (separator < 0 || separator == addr.Length - 1)
        {
            return defaultPort;
        }

        var port = addr[(separator + 1)..];
        return ushort.TryParse(port, out _) ? port : defaultPort;
    }

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

        // Checked first: a wrong base URL does not surface as a connection error, it
        // surfaces later as WebFinger links and OAuth redirects pointing somewhere
        // nobody can reach.
        var baseUrl = BaseUrl.Resolve(config[EnvVars.BaseUrl]);
        if (BaseUrl.TryValidate(baseUrl, out var baseUrlError))
        {
            Console.WriteLine($"[ok]   base URL:   {baseUrl}");
        }
        else
        {
            ok = false;
            Console.WriteLine($"[FAIL] base URL:   {baseUrl} — {baseUrlError}");
        }

        if (config.GetValue(EnvVars.TrustProxy, false))
        {
            Console.WriteLine("[ok]   proxy:      trusting X-Forwarded-* headers");
        }

        // Checked here rather than at startup alone, because the failure this guards against
        // is silent: an unreadable or mismatched certificate pair otherwise surfaces as a
        // failed handshake for every client, with nothing wrong in the logs at boot.
        if (!TlsOptions.TryResolve(
                config[EnvVars.TlsMode],
                config[EnvVars.TlsCert],
                config[EnvVars.TlsKey],
                out var tls,
                out var tlsError))
        {
            ok = false;
            Console.WriteLine($"[FAIL] tls:        {tlsError}");
        }
        else if (!tls.Enabled)
        {
            Console.WriteLine("[ok]   tls:        off — serving plain HTTP, terminate TLS at a reverse proxy");
        }
        else
        {
            var (certificatePath, keyPath) = tls.RequirePaths();
            try
            {
                // Loading proves the key matches the certificate and is readable by this user.
                using var certificate = TlsCertificate.Load(certificatePath, keyPath);
                var expiry = certificate.NotAfter.ToUniversalTime();
                var remaining = expiry - DateTime.UtcNow;

                if (remaining <= TimeSpan.Zero)
                {
                    ok = false;
                    Console.WriteLine(
                        $"[FAIL] tls:        {certificatePath} — certificate expired {expiry:u}");
                }
                else
                {
                    Console.WriteLine(
                        $"[ok]   tls:        {certificate.Subject} — expires {expiry:u} "
                        + $"({(int)remaining.TotalDays}d)");
                }
            }
            catch (Exception ex)
            {
                ok = false;
                Console.WriteLine($"[FAIL] tls:        {certificatePath} — {ex.Message}");
            }
        }

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
            // Every wired backend probes: the local ones round-trip a scratch blob,
            // Azure Blob checks container reachability and credentials.
            if (store is IStorageProbe probe)
            {
                await probe.ProbeAsync();
            }

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

    /// <summary>
    /// Populates an account with a spread of sample modules, folders, and documents
    /// (handy for exercising the file browser and dashboard). Writes through the same
    /// <see cref="RemoteStorageService"/> path the storage API uses. If no username is
    /// given, seeds the first account. Re-running overwrites the same documents.
    /// </summary>
    public static async Task<int> SeedAsync(IConfiguration config, string? username)
    {
        var databaseDsn = config["RSTASH_DB"] ?? "sqlite:rstash.sqlite";
        var blobDsn = config["RSTASH_BLOB"] ?? "sqlite:rstash-blobs.sqlite";

        var services = new ServiceCollection();
        services.AddDbContextFactory<RstashDbContext>(options => options.UseRstashDatabase(databaseDsn));
        services.AddSingleton<IStorage>(_ => StorageFactory.Open(blobDsn));
        services.AddSingleton<SettingsService>();
        services.AddSingleton<RemoteStorageService>();
        await using var provider = services.BuildServiceProvider();

        var factory = provider.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await factory.CreateDbContextAsync();

        ApplicationUser? user;
        if (!string.IsNullOrWhiteSpace(username))
        {
            var normalized = username.ToUpperInvariant();
            user = await db.Users.FirstOrDefaultAsync(u => u.NormalizedUserName == normalized);
            if (user is null)
            {
                Console.WriteLine($"[FAIL] no account named '{username}'.");
                return 1;
            }
        }
        else
        {
            user = await db.Users.OrderBy(u => u.Id).FirstOrDefaultAsync();
            if (user is null)
            {
                Console.WriteLine("[FAIL] no accounts exist yet — create one via /setup first.");
                return 1;
            }
        }

        await provider.GetRequiredService<SettingsService>().ReloadAsync();
        var storage = provider.GetRequiredService<RemoteStorageService>();

        Console.WriteLine($"Seeding sample data for '{user.UserName}' (id {user.Id})…");
        var written = 0;
        long totalBytes = 0;
        foreach (var (path, contentType, content) in SeedItems())
        {
            await using var stream = new MemoryStream(content);
            try
            {
                await storage.PutDocumentAsync(user.Id, path, stream, contentType, new StorageConditions());
                Console.WriteLine($"  + {path} ({content.Length:N0} B)");
                written++;
                totalBytes += content.Length;
            }
            catch (StorageException ex)
            {
                Console.WriteLine($"  ! {path} — {ex.Error}");
            }
        }

        Console.WriteLine($"\nDone: {written} documents, {totalBytes:N0} bytes.");
        return 0;
    }

    /// <summary>The sample dataset: several remoteStorage modules with nested folders and
    /// varied content types/sizes, including a couple of public documents.</summary>
    private static IEnumerable<(string Path, string ContentType, byte[] Content)> SeedItems()
    {
        static byte[] Text(string value) => Encoding.UTF8.GetBytes(value);

        // Deterministic filler for "binary" assets we can't ship for real (photos/media);
        // only the byte count matters for exercising sizes in the UI.
        static byte[] Filler(int bytes)
        {
            var buffer = new byte[bytes];
            for (var i = 0; i < bytes; i++)
            {
                buffer[i] = (byte)(32 + (i % 95));
            }

            return buffer;
        }

        yield return ("/documents/welcome.md", "text/markdown",
            Text("# Welcome to rstash\n\nThis document lives in your personal storage. Any\n"
                + "remoteStorage app you connect can read and write here with your\npermission.\n"));
        yield return ("/documents/notes/todo.md", "text/markdown",
            Text("# To do\n\n- [x] Set up rstash\n- [ ] Connect a remoteStorage app\n- [ ] Tell a friend\n"));
        yield return ("/documents/notes/ideas.md", "text/markdown",
            Text("# Ideas\n\n- A recipe app backed by remoteStorage\n- Sync my bookmarks everywhere\n"));
        yield return ("/documents/reports/2026-q1.txt", "text/plain",
            Filler(4_200));

        yield return ("/bookmarks/links/3f9a2c.json", "application/json",
            Text("{\"url\":\"https://remotestorage.io\",\"title\":\"remoteStorage\",\"tags\":[\"spec\"]}"));
        yield return ("/bookmarks/links/8b1e07.json", "application/json",
            Text("{\"url\":\"https://rstash.cloud\",\"title\":\"rstash\",\"tags\":[\"self-hosted\"]}"));
        yield return ("/bookmarks/archive/a4c910.json", "application/json",
            Text("{\"url\":\"https://example.com\",\"title\":\"Example\",\"tags\":[\"archive\"]}"));

        yield return ("/tasks/today.json", "application/json",
            Text("[{\"text\":\"Review PRs\",\"done\":false},{\"text\":\"Water plants\",\"done\":true}]"));
        yield return ("/tasks/someday.json", "application/json",
            Text("[{\"text\":\"Learn to sail\",\"done\":false}]"));

        yield return ("/recipes/pancakes.md", "text/markdown",
            Text("# Pancakes\n\nFlour, milk, eggs, a pinch of salt. Whisk, rest, fry.\n"));
        yield return ("/recipes/curry.md", "text/markdown",
            Text("# Weeknight curry\n\nOnion, garlic, ginger, spices, coconut milk, veg.\n"));

        yield return ("/photos/2026/hawaii-01.jpg", "image/jpeg", Filler(220_000));
        yield return ("/photos/2026/hawaii-02.jpg", "image/jpeg", Filler(180_000));
        yield return ("/photos/2026/sunset.jpg", "image/jpeg", Filler(140_000));
        yield return ("/media/intro-clip.mp4", "video/mp4", Filler(340_000));

        yield return ("/public/shared/readme.md", "text/markdown",
            Text("# Shared\n\nDocuments under /public/ are readable without a token.\n"));
        yield return ("/public/shared/logo-note.txt", "text/plain",
            Text("Anyone with the link can read this file.\n"));
    }
}
