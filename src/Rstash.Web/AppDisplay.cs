namespace Rstash.Web;

/// <summary>
/// Presentation helpers for connected remoteStorage apps (OAuth tokens), shared by
/// the dashboard card and the dedicated app-access page.
/// </summary>
public static class AppDisplay
{
    /// <summary>The granted scopes, split from their space-separated storage form.</summary>
    public static IEnumerable<string> Scopes(string scopes) =>
        scopes.Split(' ', StringSplitOptions.RemoveEmptyEntries);

    /// <summary>A human label for a client_id (typically an app origin → its host).</summary>
    public static string AppName(string clientId)
    {
        if (Uri.TryCreate(clientId, UriKind.Absolute, out var uri))
        {
            return uri.Host;
        }

        return string.IsNullOrWhiteSpace(clientId) ? "Unknown app" : clientId;
    }

    /// <summary>Up to two initials for the app's avatar tile.</summary>
    public static string AppInitials(string clientId)
    {
        var name = AppName(clientId);
        var letters = new string(name.Where(char.IsLetterOrDigit).Take(2).ToArray());
        return letters.Length == 0 ? "?" : letters.ToUpperInvariant();
    }

    /// <summary>A coarse "connected ago" label.</summary>
    public static string Relative(DateTimeOffset when)
    {
        var span = DateTimeOffset.UtcNow - when;
        if (span.TotalDays >= 14) return $"{(int)(span.TotalDays / 7)} weeks ago";
        if (span.TotalDays >= 2) return $"{(int)span.TotalDays} days ago";
        if (span.TotalDays >= 1) return "yesterday";
        if (span.TotalHours >= 1) return $"{(int)span.TotalHours} hours ago";
        return "today";
    }
}
