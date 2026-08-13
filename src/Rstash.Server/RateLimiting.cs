using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using System.Threading.RateLimiting;
using Rstash.Services;

namespace Rstash.Server;

/// <summary>
/// Per-IP and per-user request throttling, so one client can't exhaust the
/// server and password guessing costs an attacker real time.
/// </summary>
/// <remarks>
/// Three scopes, because one budget cannot serve them all: signing in should be
/// stingy (a person types one password at a time), the storage API should be
/// generous (a sync client legitimately fires a burst of small writes), and
/// everything else sits in between. Static assets and health checks are never
/// limited — throttling them breaks the UI and hides whether the server is up.
/// </remarks>
public static class RateLimiting
{
    /// <summary>
    /// Endpoints where the secret being presented is one a human chose, and so is
    /// worth guessing. <c>/oauth/token</c> is deliberately absent: the codes and
    /// refresh tokens it accepts are 256-bit random values that no amount of
    /// guessing will find, while a household running several apps legitimately
    /// redeems tokens far faster than a person types passwords.
    /// </summary>
    private static readonly string[] AuthPaths =
        ["/login", "/register", "/setup", "/forgot-password", "/reset-password"];

    /// <summary>
    /// Extensions served out of <c>wwwroot</c>. Only ever consulted for
    /// root-level paths, so a stored document named <c>cat.png</c> can't buy
    /// itself an exemption from the storage budget.
    /// </summary>
    private static readonly string[] StaticExtensions =
        [".css", ".js", ".mjs", ".map", ".ico", ".svg", ".png", ".jpg", ".jpeg",
         ".gif", ".webp", ".woff", ".woff2", ".ttf", ".webmanifest", ".txt"];

    /// <summary>
    /// Ceiling on a token bucket's replenishment period. A rate of 1e-9/s is a
    /// valid setting write, and <c>TimeSpan.FromSeconds(1/rate)</c> would
    /// overflow — on the global limiter, so every request 500s, including the
    /// settings page you'd use to undo it.
    /// </summary>
    private const double MaxPeriodSeconds = 86_400;

    public static IServiceCollection AddRstashRateLimiting(this IServiceCollection services)
    {
        services.AddRateLimiter(options =>
        {
            options.OnRejected = async (context, cancellationToken) =>
            {
                context.HttpContext.Response.StatusCode = StatusCodes.Status429TooManyRequests;

                if (context.Lease.TryGetMetadata(MetadataName.RetryAfter, out var retryAfter))
                {
                    context.HttpContext.Response.Headers.RetryAfter =
                        ((int)retryAfter.TotalSeconds).ToString(CultureInfo.InvariantCulture);
                }

                await context.HttpContext.Response.WriteAsync("Too many requests.", cancellationToken);
            };

            options.GlobalLimiter = PartitionedRateLimiter.Create<HttpContext, string>(Partition);
        });

        return services;
    }

    private static RateLimitPartition<string> Partition(HttpContext context)
    {
        var settings = context.RequestServices.GetRequiredService<SettingsService>().Current;
        var path = context.Request.Path;

        if (!settings.RateLimit || IsExempt(path))
        {
            return RateLimitPartition.GetNoLimiter("none");
        }

        // Only the submission is charged to the stingy budget. Rendering the form
        // costs a token too if you count every method, and at 0.2/s that means a
        // GET plus four bad passwords exhausts the bucket — so the fifth attempt,
        // the one that trips the account lockout, returns a bare 429 and the user
        // never learns their account is locked.
        if (IsAuthPath(path) && !HttpMethods.IsGet(context.Request.Method))
        {
            return Bucket("auth", ClientKey(context), settings.AuthRateLimitRate, settings.AuthRateLimitBurst);
        }

        // Storage traffic is charged to the app token, not the address: several
        // people behind one home IP shouldn't share a sync budget. It can't be
        // charged to the account — /storage authenticates by reading the bearer
        // header inside the endpoint, so there is no authentication scheme and
        // context.User is still anonymous this early in the pipeline.
        if (path.StartsWithSegments("/storage") && BearerKey(context) is { } bearer)
        {
            return Bucket("token", bearer, settings.UserRateLimitRate, settings.UserRateLimitBurst);
        }

        return Bucket("ip", ClientKey(context), settings.RateLimitRate, settings.RateLimitBurst);
    }

    /// <summary>Static assets, health checks, and the Blazor circuit are never throttled.</summary>
    private static bool IsExempt(PathString path) =>
        path.StartsWithSegments("/_framework")
        || path.StartsWithSegments("/_content")
        || path.StartsWithSegments("/_blazor")
        || path.StartsWithSegments("/healthz")
        || IsStaticAsset(path);

    /// <summary>
    /// True for the files this app actually ships — <c>app.css</c>, <c>app.js</c>,
    /// the favicons, the manifest — which sit at the root of <c>wwwroot</c>, not
    /// under <c>/css</c> or <c>/js</c>. Matching on directories that don't exist
    /// meant every page load spent five tokens of the caller's request budget,
    /// and a throttled stylesheet renders the UI unstyled rather than erroring.
    /// </summary>
    private static bool IsStaticAsset(PathString path)
    {
        var value = path.Value;

        // Root-level only: a nested path is /storage/… or a page, never an asset.
        // MapStaticAssets' fingerprinted names (app.6f3a1b.css) stay single-segment.
        if (string.IsNullOrEmpty(value) || value.IndexOf('/', 1) >= 0)
        {
            return false;
        }

        var dot = value.LastIndexOf('.');
        return dot > 0 && StaticExtensions.Contains(value[dot..], StringComparer.OrdinalIgnoreCase);
    }

    private static bool IsAuthPath(PathString path) =>
        AuthPaths.Any(p => path.StartsWithSegments(p));

    private static string ClientKey(HttpContext context) =>
        context.Connection.RemoteIpAddress?.ToString() ?? "unknown";

    /// <summary>
    /// A stable per-token key, or null when the request carries no bearer token.
    /// Hashed because the partition key outlives the request inside the limiter's
    /// dictionary, and a bearer secret has no business living there.
    /// </summary>
    private static string? BearerKey(HttpContext context)
    {
        var header = context.Request.Headers.Authorization.ToString();
        if (!header.StartsWith("Bearer ", StringComparison.Ordinal))
        {
            return null;
        }

        var token = header["Bearer ".Length..];
        if (token.Length == 0)
        {
            return null;
        }

        return Convert.ToHexStringLower(SHA256.HashData(Encoding.UTF8.GetBytes(token)).AsSpan(0, 16));
    }

    /// <summary>
    /// A token bucket for one scope, or no limit when the rate is 0.
    /// </summary>
    /// <remarks>
    /// The rate and burst are part of the partition key so that editing a setting
    /// takes effect immediately: a changed value produces a new key, and with it a
    /// limiter built from the new numbers. Keying on the values alone would leave
    /// every existing client pinned to whatever was configured when they first
    /// arrived, and a rate limit you cannot lower during an incident is not much
    /// of a rate limit.
    /// </remarks>
    private static RateLimitPartition<string> Bucket(string scope, string key, double rate, int burst)
    {
        if (rate <= 0 || burst <= 0)
        {
            return RateLimitPartition.GetNoLimiter("none");
        }

        // Whole rates replenish once a second; fractional ones (e.g. 0.2/s, a
        // sign-in every five seconds) get a single token on a longer period.
        var (tokensPerPeriod, period) = rate >= 1
            ? ((int)Math.Round(rate), TimeSpan.FromSeconds(1))
            : (1, TimeSpan.FromSeconds(Math.Min(1 / rate, MaxPeriodSeconds)));

        return RateLimitPartition.GetTokenBucketLimiter(
            $"{scope}:{key}:{rate}:{burst}",
            _ => new TokenBucketRateLimiterOptions
            {
                TokenLimit = burst,
                TokensPerPeriod = tokensPerPeriod,
                ReplenishmentPeriod = period,
                QueueLimit = 0,
                AutoReplenishment = true,
            });
    }

    /// <summary>
    /// Warns when per-IP limiting is on but every request will arrive wearing the
    /// same address, which buckets the whole household together and locks them
    /// out as one.
    /// </summary>
    public static void WarnIfProxiedWithoutTrust(ILogger logger, bool trustProxy, SettingsSnapshot settings)
    {
        if (settings.RateLimit && !trustProxy)
        {
            logger.LogInformation(
                "Rate limiting is on and RSTASH_TRUST_PROXY is off. If rstash is behind a reverse "
                + "proxy, set RSTASH_TRUST_PROXY=true — otherwise every request appears to come "
                + "from the proxy and all clients share a single rate-limit budget.");
        }
    }
}
