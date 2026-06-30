namespace Rstash.Database;

/// <summary>
/// A persisted runtime-settings override (key → value). Absence of a row means
/// the setting uses its registry/env default.
/// </summary>
public class Setting
{
    public string Key { get; set; } = string.Empty;

    public string Value { get; set; } = string.Empty;

    public DateTimeOffset UpdatedAt { get; set; }
}
