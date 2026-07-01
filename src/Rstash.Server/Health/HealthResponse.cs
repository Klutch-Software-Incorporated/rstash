using System.Text.Json;
using Microsoft.Extensions.Diagnostics.HealthChecks;

namespace Rstash.Server.Health;

/// <summary>
/// Writes <c>/healthz</c> as JSON with the overall status plus a per-dependency
/// breakdown, so a failing database or storage backend is visible by name.
/// Only friendly descriptions are emitted — never exception details.
/// </summary>
internal static class HealthResponse
{
    public static Task WriteAsync(HttpContext context, HealthReport report)
    {
        context.Response.ContentType = "application/json";

        var payload = new
        {
            status = report.Status.ToString(),
            checks = report.Entries.Select(entry => new
            {
                name = entry.Key,
                status = entry.Value.Status.ToString(),
                description = entry.Value.Description,
            }),
        };

        return context.Response.WriteAsync(JsonSerializer.Serialize(payload));
    }
}
