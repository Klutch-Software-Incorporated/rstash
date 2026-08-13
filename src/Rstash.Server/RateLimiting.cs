using System.Globalization;
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

        if (IsAuthPath(path))
        {
            return Bucket("auth", ClientKey(context), settings.AuthRateLimitRate, settings.AuthRateLimitBurst);
        }

        // Storage traffic is charged to the account, not the address: several
        // people behind one home IP shouldn't share a sync budget.
        if (path.StartsWithSegments("/storage") && context.User.Identity?.IsAuthenticated == true)
        {
            var user = context.User.Identity.Name ?? "anonymous";
            return Bucket("user", user, settings.UserRateLimitRate, settings.UserRateLimitBurst);
        }

        return Bucket("ip", ClientKey(context), settings.RateLimitRate, settings.RateLimitBurst);
    }

    /// <summary>Static assets, health checks, and the Blazor circuit are never throttled.</summary>
    private static bool IsExempt(PathString path) =>
        path.StartsWithSegments("/_framework")
        || path.StartsWithSegments("/_content")
        || path.StartsWithSegments("/_blazor")
        || path.StartsWithSegments("/healthz")
        || path.StartsWithSegments("/css")
        || path.StartsWithSegments("/js")
        || path.StartsWithSegments("/lib")
        || path.StartsWithSegments("/favicon.ico");

    private static bool IsAuthPath(PathString path) =>
        AuthPaths.Any(p => path.StartsWithSegments(p));

    private static string ClientKey(HttpContext context) =>
        context.Connection.RemoteIpAddress?.ToString() ?? "unknown";

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
            : (1, TimeSpan.FromSeconds(1 / rate));

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
