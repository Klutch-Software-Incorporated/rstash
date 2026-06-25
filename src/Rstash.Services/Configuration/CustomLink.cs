using System.Text.Json;
using System.Text.Json.Serialization;

namespace Rstash.Services.Configuration;

/// <summary>An admin-configured external link shown in the user menu / profile.</summary>
public sealed record CustomLink
{
    [JsonPropertyName("label")]
    public string Label { get; init; } = "";

    [JsonPropertyName("url")]
    public string Url { get; init; } = "";

    [JsonPropertyName("description")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Description { get; init; }
}

/// <summary>Parsing and validation for the <c>custom_links</c> JSON setting.</summary>
public static class CustomLinks
{
    public const int MaxLinks = 10;

    /// <summary>
    /// Parses and validates the custom_links JSON array. Throws
    /// <see cref="SettingValidationException"/> on malformed input.
    /// </summary>
    public static IReadOnlyList<CustomLink> Parse(string raw)
    {
        raw = raw.Trim();
        if (raw.Length == 0)
        {
            return [];
        }

        List<CustomLink>? links;
        try
        {
            links = JsonSerializer.Deserialize<List<CustomLink>>(raw);
        }
        catch (JsonException ex)
        {
            throw new SettingValidationException($"invalid JSON: {ex.Message}");
        }

        links ??= [];
        if (links.Count > MaxLinks)
        {
            throw new SettingValidationException($"too many links (max {MaxLinks})");
        }

        for (var i = 0; i < links.Count; i++)
        {
            if (string.IsNullOrWhiteSpace(links[i].Label))
            {
                throw new SettingValidationException($"link {i + 1}: label is required");
            }

            ValidateLinkUrl(links[i].Url, i + 1, links[i].Label);
        }

        return links;
    }

    public static bool TryParse(string raw, out IReadOnlyList<CustomLink> links)
    {
        try
        {
            links = Parse(raw);
            return true;
        }
        catch (SettingValidationException)
        {
            links = [];
            return false;
        }
    }

    // Accepts https:// absolute URLs or relative paths starting with '/'.
    // Other schemes are rejected to avoid javascript:/data: injection or
    // accidental http:// in a TLS deployment.
    private static void ValidateLinkUrl(string raw, int index, string label)
    {
        if (raw.StartsWith('/'))
        {
            return;
        }

        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri))
        {
            throw new SettingValidationException($"link {index} (\"{label}\"): invalid URL");
        }

        if (uri.Scheme != Uri.UriSchemeHttps)
        {
            throw new SettingValidationException(
                $"link {index} (\"{label}\"): URL must be https:// or a relative path starting with /");
        }

        if (string.IsNullOrEmpty(uri.Host))
        {
            throw new SettingValidationException($"link {index} (\"{label}\"): URL must include a host");
        }
    }
}
