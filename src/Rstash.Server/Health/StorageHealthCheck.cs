using Microsoft.Extensions.Diagnostics.HealthChecks;
using Rstash.Storage;

namespace Rstash.Server.Health;

/// <summary>
/// Reports the blob storage backend as healthy only if it can be written to and
/// read back. Every wired backend — filesystem, database, Azure Blob — implements
/// <see cref="IStorageProbe"/>; the fallback covers one that doesn't.
/// </summary>
/// <remarks>
/// The result comes through <see cref="StorageProbeCache"/> rather than straight
/// off the backend, so the cost of this check is bounded no matter how often it
/// is polled.
/// </remarks>
internal sealed class StorageHealthCheck(IStorage storage, StorageProbeCache cache) : IHealthCheck
{
    public async Task<HealthCheckResult> CheckHealthAsync(
        HealthCheckContext context, CancellationToken cancellationToken = default)
    {
        if (storage is not IStorageProbe probe)
        {
            return HealthCheckResult.Healthy("Storage backend requires no connectivity probe.");
        }

        return await cache.GetAsync(async ct =>
        {
            try
            {
                await probe.ProbeAsync(ct);
                return HealthCheckResult.Healthy("Storage backend reachable.");
            }
            catch (Exception ex)
            {
                return HealthCheckResult.Unhealthy("Storage backend cannot be reached.", ex);
            }
        }, cancellationToken);
    }
}
