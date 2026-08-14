using Rstash.Server.Endpoints;

namespace Rstash.IntegrationTests;

/// <summary>
/// Retry-After points at the end of the current metering period. A unit test rather
/// than an endpoint one, because the interesting cases are calendar edges that a live
/// request can only exercise on the right day of the year.
/// </summary>
public sealed class EgressLimitTests
{
    [Fact]
    public void RetryAfter_CountsToTheNextMonthBoundary()
    {
        var now = new DateTimeOffset(2026, 8, 14, 12, 0, 0, TimeSpan.Zero);

        // 17 days and 12 hours until 1 September.
        Assert.Equal((17 * 24 + 12) * 3600, EgressLimit.RetryAfterSeconds(now));
    }

    [Fact]
    public void RetryAfter_RollsOverTheYearInDecember()
    {
        var now = new DateTimeOffset(2026, 12, 31, 23, 0, 0, TimeSpan.Zero);

        Assert.Equal(3600, EgressLimit.RetryAfterSeconds(now));
    }

    [Fact]
    public void RetryAfter_IsNeverZero()
    {
        // The instant the period ends, the answer is "try again now", not "already".
        // Retry-After: 0 is legal but reads as an invitation to hammer the door.
        var boundary = new DateTimeOffset(2026, 9, 1, 0, 0, 0, TimeSpan.Zero);

        Assert.Equal(1, EgressLimit.RetryAfterSeconds(boundary.AddSeconds(-0.5)));
    }
}
