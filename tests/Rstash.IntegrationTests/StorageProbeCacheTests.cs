using Microsoft.Extensions.Diagnostics.HealthChecks;
using Rstash.Server.Health;

namespace Rstash.IntegrationTests;

/// <summary>
/// /healthz is anonymous and exempt from rate limiting, and the storage probe
/// behind it is a real write/read/delete against the blob store. Without a cache
/// that combination is an unauthenticated way to drive unbounded writes.
/// </summary>
public sealed class StorageProbeCacheTests
{
    [Fact]
    public async Task RepeatedChecks_WithinTheWindow_ProbeOnce()
    {
        var clock = new StoppedClock(DateTimeOffset.UnixEpoch);
        var cache = new StorageProbeCache(clock);
        var probes = 0;

        for (var i = 0; i < 50; i++)
        {
            await cache.GetAsync(_ => Probe(ref probes), CancellationToken.None);
        }

        Assert.Equal(1, probes);
    }

    [Fact]
    public async Task ChecksAfterTheWindow_ProbeAgain()
    {
        var clock = new StoppedClock(DateTimeOffset.UnixEpoch);
        var cache = new StorageProbeCache(clock);
        var probes = 0;

        await cache.GetAsync(_ => Probe(ref probes), CancellationToken.None);
        clock.Advance(TimeSpan.FromSeconds(11));
        await cache.GetAsync(_ => Probe(ref probes), CancellationToken.None);

        Assert.Equal(2, probes);
    }

    /// <summary>
    /// A stale "healthy" is the failure mode that matters — a backend that died
    /// inside the window must not keep being reported up once the window passes.
    /// </summary>
    [Fact]
    public async Task AFailureAfterTheWindow_ReplacesTheCachedSuccess()
    {
        var clock = new StoppedClock(DateTimeOffset.UnixEpoch);
        var cache = new StorageProbeCache(clock);

        var healthy = await cache.GetAsync(
            _ => Task.FromResult(HealthCheckResult.Healthy()), CancellationToken.None);
        Assert.Equal(HealthStatus.Healthy, healthy.Status);

        clock.Advance(TimeSpan.FromSeconds(11));

        var afterwards = await cache.GetAsync(
            _ => Task.FromResult(HealthCheckResult.Unhealthy()), CancellationToken.None);
        Assert.Equal(HealthStatus.Unhealthy, afterwards.Status);
    }

    /// <summary>
    /// A burst of simultaneous polls — what a load balancer plus a monitor plus a
    /// curious caller looks like — must still cost one round-trip, not one each.
    /// </summary>
    [Fact]
    public async Task ConcurrentChecks_ShareASingleProbe()
    {
        var clock = new StoppedClock(DateTimeOffset.UnixEpoch);
        var cache = new StorageProbeCache(clock);
        var probes = 0;

        await Task.WhenAll(Enumerable.Range(0, 32).Select(_ => Task.Run(async () =>
            await cache.GetAsync(
                async ct =>
                {
                    Interlocked.Increment(ref probes);
                    await Task.Delay(5, ct);
                    return HealthCheckResult.Healthy();
                },
                CancellationToken.None))));

        Assert.Equal(1, probes);
    }

    private static Task<HealthCheckResult> Probe(ref int counter)
    {
        Interlocked.Increment(ref counter);
        return Task.FromResult(HealthCheckResult.Healthy());
    }

    private sealed class StoppedClock(DateTimeOffset start) : TimeProvider
    {
        private DateTimeOffset _now = start;

        public override DateTimeOffset GetUtcNow() => _now;

        public void Advance(TimeSpan by) => _now = _now.Add(by);
    }
}
