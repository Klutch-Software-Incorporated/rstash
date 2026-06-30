using System.Globalization;
using System.Text.RegularExpressions;

namespace Rstash.Services.Configuration;

/// <summary>
/// Parses a token-lifetime string into a <see cref="TimeSpan"/>. Accepts Go-style
/// duration strings (e.g. "720h", "30m", "1h30m") plus a convenience "d" day
/// suffix (e.g. "30d"). "" or "0" means no expiry (<see cref="TimeSpan.Zero"/>).
/// </summary>
public static partial class TokenLifetime
{
    public static TimeSpan Parse(string value)
    {
        var s = value.Trim();
        if (s.Length == 0 || s == "0")
        {
            return TimeSpan.Zero;
        }

        // Convenience "d" suffix: a whole number of days.
        if (s.EndsWith('d') || s.EndsWith('D'))
        {
            var daysPart = s[..^1].Trim();
            if (!long.TryParse(daysPart, NumberStyles.Integer, CultureInfo.InvariantCulture, out var days)
                || days < 0)
            {
                throw new FormatException($"invalid token lifetime: \"{value}\"");
            }

            return TimeSpan.FromDays(days);
        }

        var body = s;
        if (body.StartsWith('+'))
        {
            body = body[1..];
        }
        else if (body.StartsWith('-'))
        {
            throw new FormatException($"negative token lifetime: \"{value}\"");
        }

        if (!GoDurationRegex().IsMatch(body))
        {
            throw new FormatException($"invalid token lifetime: \"{value}\"");
        }

        var seconds = 0d;
        foreach (Match segment in GoDurationSegmentRegex().Matches(body))
        {
            var n = double.Parse(segment.Groups[1].Value, CultureInfo.InvariantCulture);
            seconds += n * UnitSeconds(segment.Groups[2].Value);
        }

        return TimeSpan.FromSeconds(seconds);
    }

    private static double UnitSeconds(string unit) => unit switch
    {
        "ns" => 1e-9,
        "us" or "µs" => 1e-6,
        "ms" => 1e-3,
        "s" => 1,
        "m" => 60,
        "h" => 3600,
        _ => throw new FormatException($"unknown duration unit: \"{unit}\""),
    };

    [GeneratedRegex(@"^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$", RegexOptions.CultureInvariant)]
    private static partial Regex GoDurationRegex();

    [GeneratedRegex(@"(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)", RegexOptions.CultureInvariant)]
    private static partial Regex GoDurationSegmentRegex();
}
