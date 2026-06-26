namespace Rstash.Server.Endpoints;

/// <summary>WebFinger discovery for remoteStorage (draft-dejong-remotestorage-26).</summary>
internal static class WellKnownEndpoints
{
    public static void MapWebFinger(this IEndpointRouteBuilder endpoints, string baseUrl)
    {
        endpoints.MapGet("/.well-known/webfinger", (string? resource) =>
        {
            if (string.IsNullOrEmpty(resource))
            {
                return Results.BadRequest("missing resource parameter");
            }

            if (!resource.StartsWith("acct:", StringComparison.Ordinal))
            {
                return Results.BadRequest("invalid resource format");
            }

            var rest = resource["acct:".Length..];
            var at = rest.IndexOf('@', StringComparison.Ordinal);
            if (at < 0)
            {
                return Results.BadRequest("invalid resource format");
            }

            var username = rest[..at].ToLowerInvariant();
            var host = rest[(at + 1)..];

            var authUrl = $"{baseUrl}/oauth/authorize";
            var tokenUrl = $"{baseUrl}/oauth/token";

            var document = new
            {
                subject = $"acct:{username}@{host}",
                links = new[]
                {
                    new
                    {
                        href = $"{baseUrl}/storage/{username}",
                        rel = "http://tools.ietf.org/id/draft-dejong-remotestorage",
                        type = "draft-dejong-remotestorage-26",
                        properties = new Dictionary<string, string>
                        {
                            ["http://remotestorage.io/spec/version"] = "draft-dejong-remotestorage-26",
                            ["http://tools.ietf.org/html/rfc6749#section-4.2"] = authUrl,
                            ["http://tools.ietf.org/html/rfc6749#section-3.1"] = authUrl,
                            ["http://tools.ietf.org/html/rfc6749#section-3.2"] = tokenUrl,
                            ["http://tools.ietf.org/html/rfc7636"] = "S256",
                        },
                    },
                },
            };

            return Results.Json(document, contentType: "application/jrd+json");
        }).RequireCors("rstash-storage");
    }
}
