using System.Text.RegularExpressions;

namespace Rstash.Model;

/// <summary>
/// remoteStorage OAuth scopes: "module:r" / "module:rw" (module is
/// [A-Za-z0-9_-]+ or "*").
/// </summary>
public static partial class Scope
{
    [GeneratedRegex(@"^([a-zA-Z0-9_-]+|\*):r(w)?$")]
    private static partial Regex ScopePattern();

    /// <summary>
    /// Validates a space-separated scope string. "public" is reserved per
    /// draft-dejong-remotestorage-26 and rejected as a module name.
    /// </summary>
    public static bool TryParse(string scopeString, out IReadOnlyList<string> scopes)
    {
        var parts = scopeString.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0)
        {
            scopes = [];
            return false;
        }

        foreach (var part in parts)
        {
            if (!ScopePattern().IsMatch(part))
            {
                scopes = [];
                return false;
            }

            var module = part[..part.IndexOf(':', StringComparison.Ordinal)];
            if (module == "public")
            {
                scopes = [];
                return false;
            }
        }

        scopes = parts;
        return true;
    }

    /// <summary>
    /// True if the scopes grant the requested access to a storage path. The
    /// module is the first path segment (paths under "public/&lt;module&gt;"
    /// belong to &lt;module&gt;). "rw" grants read+write; "r" grants read only.
    /// </summary>
    public static bool Grants(IReadOnlyList<string> scopes, string storagePath, bool write)
    {
        var trimmed = storagePath.TrimStart('/');
        if (trimmed.StartsWith("public/", StringComparison.Ordinal))
        {
            trimmed = trimmed["public/".Length..];
        }

        var module = trimmed;
        var slash = trimmed.IndexOf('/', StringComparison.Ordinal);
        if (slash >= 0)
        {
            module = trimmed[..slash];
        }

        foreach (var scope in scopes)
        {
            var match = ScopePattern().Match(scope);
            if (!match.Success)
            {
                continue;
            }

            var scopeModule = match.Groups[1].Value;
            var hasWrite = match.Groups[2].Value == "w";

            if (scopeModule != "*" && scopeModule != module)
            {
                continue;
            }

            if (write && !hasWrite)
            {
                continue;
            }

            return true;
        }

        return false;
    }
}
