using Microsoft.Extensions.Diagnostics.HealthChecks;
using Rstash.Storage;

namespace Rstash.Server.Health;

/// <summary>
/// Reports the blob storage backend as healthy only if it can be reached. Remote
/// backends (e.g. Azure Blob) expose <see cref="IStorageProbe"/>; local backends
/// (filesystem, SQLite) validate on open and need no connectivity probe.
/// </summary>
internal sealed class StorageHealthCheck(IStorage storage) : IHealthCheck
{
    public async Task<HealthCheckResult> CheckHealthAsync(
        HealthCheckContext context, CancellationToken cancellationToken = default)
    {
        if (storage is not IStorageProbe probe)
        {
            return HealthCheckResult.Healthy("Storage backend requires no connectivity probe.");
        }

        try
        {
            await probe.ProbeAsync(cancellationToken);
            return HealthCheckResult.Healthy("Storage backend reachable.");
        }
        catch (Exception ex)
        {
            return HealthCheckResult.Unhealthy("Storage backend cannot be reached.", ex);
        }
    }
}
