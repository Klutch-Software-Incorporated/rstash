namespace Rstash.Services.Configuration;

/// <summary>
/// Describes one configurable setting — the single source of truth for
/// env-file generation, admin UI rendering, and validation.
/// </summary>
public sealed record SettingDef
{
    /// <summary>Internal key, e.g. "registration_mode".</summary>
    public required string Key { get; init; }

    /// <summary>Env var name (null = no env var; runtime-only).</summary>
    public string? EnvVar { get; init; }

    /// <summary>UI grouping, e.g. "Access", "Storage".</summary>
    public string Group { get; init; } = "";

    public string Label { get; init; } = "";

    /// <summary>One-line description (grid + env file).</summary>
    public string Description { get; init; } = "";

    /// <summary>Extended docs for the detail page / env doc.</summary>
    public string Help { get; init; } = "";

    /// <summary>Default value as a display string.</summary>
    public string Default { get; init; } = "";

    /// <summary>Allowed values for selects and validation.</summary>
    public IReadOnlyList<string> ValidValues { get; init; } = [];

    public SettingInputType InputType { get; init; } = SettingInputType.Text;

    /// <summary>Whether the setting can be changed at runtime (DB-backed).</summary>
    public bool RuntimeEditable { get; init; }

    public string? NumberMin { get; init; }

    public string? NumberStep { get; init; }
}
