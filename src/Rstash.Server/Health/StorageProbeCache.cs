using Microsoft.Extensions.Diagnostics.HealthChecks;

namespace Rstash.Server.Health;

/// <summary>
/// Holds the storage probe's last outcome for a short window so that polling
/// <c>/healthz</c> doesn't turn into polling the blob store.
/// </summary>
/// <remarks>
/// The probe is a real round-trip — write, read, delete — which is what makes it
/// worth running at all, and also what makes running it per request a problem:
/// <c>/healthz</c> is anonymous and exempt from rate limiting, so an unauthenticated
/// caller could drive unbounded writes. On the SQLite-on-SMB deployment every one
/// of them takes a write lock.
/// <para>
/// Failures are cached on the same terms as successes. A backend that has just
/// come back stays reported as down for up to the window, which is well inside any
/// sensible unhealthy threshold and keeps a broken store from being hammered by
/// the very checks watching it.
/// </para>
/// <para>
/// Callers are serialized, so a burst of simultaneous polls produces one probe and
/// shares its answer rather than one probe each.
/// </para>
/// </remarks>
internal sealed class StorageProbeCache(TimeProvider clock)
{
    private static readonly TimeSpan Window = TimeSpan.FromSeconds(10);

    private readonly SemaphoreSlim _gate = new(1, 1);

    private HealthCheckResult _last;
    private DateTimeOffset? _takenAt;

    public async Task<HealthCheckResult> GetAsync(
        Func<CancellationToken, Task<HealthCheckResult>> probe,
        CancellationToken cancellationToken)
    {
        await _gate.WaitAsync(cancellationToken);
        try
        {
            var now = clock.GetUtcNow();
            if (_takenAt is { } takenAt && now - takenAt < Window)
            {
                return _last;
            }

            _last = await probe(cancellationToken);
            _takenAt = now;
            return _last;
        }
        finally
        {
            _gate.Release();
        }
    }
}
