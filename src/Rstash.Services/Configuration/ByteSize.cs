using System.Globalization;

namespace Rstash.Services.Configuration;

/// <summary>
/// Human-readable byte-size parsing and formatting. Binary units (1 KB = 1024
/// bytes); a plain integer is bytes.
/// </summary>
public static class ByteSize
{
    // Longer suffixes first so "MB"/"KB" win over the bare "B".
    private static readonly (string Suffix, long Multiplier)[] Suffixes =
    [
        ("TB", 1L << 40),
        ("GB", 1L << 30),
        ("MB", 1L << 20),
        ("KB", 1L << 10),
        ("B", 1L),
    ];

    public static long Parse(string value)
    {
        var s = value.Trim();
        if (s.Length == 0)
        {
            throw new FormatException("empty size string");
        }

        var upper = s.ToUpperInvariant();
        foreach (var (suffix, multiplier) in Suffixes)
        {
            if (!upper.EndsWith(suffix, StringComparison.Ordinal))
            {
                continue;
            }

            var numberPart = s[..^suffix.Length].Trim();
            if (numberPart.Length == 0
                || !double.TryParse(numberPart, NumberStyles.Float, CultureInfo.InvariantCulture, out var n))
            {
                throw new FormatException($"invalid size: \"{value}\"");
            }

            if (n < 0)
            {
                throw new FormatException($"negative size: \"{value}\"");
            }

            return (long)(n * multiplier);
        }

        // No suffix — raw bytes.
        if (!long.TryParse(s, NumberStyles.Integer, CultureInfo.InvariantCulture, out var bytes))
        {
            throw new FormatException($"invalid size: \"{value}\"");
        }

        if (bytes < 0)
        {
            throw new FormatException($"negative size: \"{value}\"");
        }

        return bytes;
    }

    public static bool TryParse(string value, out long bytes)
    {
        try
        {
            bytes = Parse(value);
            return true;
        }
        catch (FormatException)
        {
            bytes = 0;
            return false;
        }
    }

    public static string Format(long bytes) => bytes switch
    {
        >= 1L << 40 => Scaled(bytes, 1L << 40, "TB"),
        >= 1L << 30 => Scaled(bytes, 1L << 30, "GB"),
        >= 1L << 20 => Scaled(bytes, 1L << 20, "MB"),
        >= 1L << 10 => Scaled(bytes, 1L << 10, "KB"),
        _ => $"{bytes} B",
    };

    private static string Scaled(long bytes, long unit, string label) =>
        string.Create(CultureInfo.InvariantCulture, $"{(double)bytes / unit:0.0} {label}");
}
