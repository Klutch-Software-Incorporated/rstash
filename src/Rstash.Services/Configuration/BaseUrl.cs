using System.Diagnostics.CodeAnalysis;

namespace Rstash.Services.Configuration;

/// <summary>
/// Resolves and validates <c>RSTASH_BASE_URL</c>, the server's public origin.
/// </summary>
/// <remarks>
/// Every absolute URL rstash emits — WebFinger's storage and auth hrefs, OAuth
/// redirects, password-reset links — is built from this value rather than from the
/// incoming request. That is deliberate: a TLS-terminating reverse proxy makes the
/// app believe it lives at <c>http://localhost:8080</c> while users are at
/// <c>https://…</c>, so request-derived URLs would be wrong for exactly the
/// deployments that matter most. A malformed value is therefore a boot-time
/// failure, not a cosmetic one.
/// </remarks>
public static class BaseUrl
{
    public const string Default = "http://localhost:8080";

    /// <summary>Applies the default and strips any trailing slash, so callers can
    /// concatenate paths without doubling separators.</summary>
    public static string Resolve(string? configured) =>
        string.IsNullOrWhiteSpace(configured) ? Default : configured.Trim().TrimEnd('/');

    /// <summary>
    /// Checks that a resolved base URL can serve as an origin (and, later, an OIDC
    /// issuer). Rejects relative values, non-HTTP schemes, and anything carrying a
    /// path, query, or fragment — an issuer with trailing junk fails token
    /// validation in ways that are painful to diagnose.
    /// </summary>
    public static bool TryValidate(string resolved, [NotNullWhen(false)] out string? error)
    {
        if (!Uri.TryCreate(resolved, UriKind.Absolute, out var uri))
        {
            error = "must be an absolute URL including the scheme, e.g. https://rstash.example.com";
            return false;
        }

        if (uri.Scheme != Uri.UriSchemeHttp && uri.Scheme != Uri.UriSchemeHttps)
        {
            error = $"scheme must be http or https, not '{uri.Scheme}'";
            return false;
        }

        if (uri.AbsolutePath != "/")
        {
            error = $"must be an origin with no path segment; drop '{uri.AbsolutePath}'";
            return false;
        }

        if (uri.Query.Length > 0 || uri.Fragment.Length > 0)
        {
            error = "must not carry a query string or fragment";
            return false;
        }

        error = null;
        return true;
    }

    /// <summary>Resolve + validate for the boot path, where a bad value must stop the server.</summary>
    public static string ResolveOrThrow(string? configured)
    {
        var resolved = Resolve(configured);
        if (!TryValidate(resolved, out var error))
        {
            throw new InvalidOperationException($"{EnvVars.BaseUrl} ('{resolved}') is invalid: {error}.");
        }

        return resolved;
    }
}
