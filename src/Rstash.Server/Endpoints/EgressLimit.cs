using System.Globalization;

namespace Rstash.Server.Endpoints;

/// <summary>
/// Shared shape of the "you have used your transfer allowance" response, so the two
/// doors onto the same bytes — the storage API and the in-app file browser — answer
/// the same way when the meter is full.
/// </summary>
/// <remarks>
/// 429, not 507: egress is transfer (not "storage full"), and the limit clears when
/// the monthly period rolls over. Retry-After points at the next UTC month boundary so
/// clients and HTTP libraries back off until then.
/// (draft-dejong-remotestorage-26 §5 / RFC 6585.)
/// </remarks>
internal static class EgressLimit
{
    /// <summary>Seconds from <paramref name="now"/> until the current period ends.</summary>
    public static int RetryAfterSeconds(DateTimeOffset now)
    {
        var nextMonth = new DateTimeOffset(now.Year, now.Month, 1, 0, 0, 0, TimeSpan.Zero).AddMonths(1);
        return Math.Max(1, (int)Math.Ceiling((nextMonth - now).TotalSeconds));
    }

    /// <summary>Stamps Retry-After for the current period onto the response.</summary>
    public static void SetRetryAfter(HttpResponse response) =>
        response.Headers["Retry-After"] =
            RetryAfterSeconds(DateTimeOffset.UtcNow).ToString(CultureInfo.InvariantCulture);
}
