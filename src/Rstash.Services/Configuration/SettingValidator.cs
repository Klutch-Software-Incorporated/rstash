using System.Globalization;

namespace Rstash.Services.Configuration;

/// <summary>
/// Validates a setting write against the registry: the key must be known and
/// runtime-editable, and the value must satisfy the setting's input type.
/// </summary>
public static class SettingValidator
{
    /// <summary>Throws <see cref="SettingValidationException"/> if the write is invalid.</summary>
    public static void Validate(string key, string value)
    {
        var def = SettingDefinitions.ByKey(key)
            ?? throw new SettingValidationException($"unknown setting: \"{key}\"");

        if (!def.RuntimeEditable)
        {
            throw new SettingValidationException($"setting \"{key}\" cannot be changed at runtime");
        }

        switch (def.InputType)
        {
            case SettingInputType.Select:
                if (!def.ValidValues.Contains(value, StringComparer.Ordinal))
                {
                    throw new SettingValidationException(
                        $"{key} must be one of: {string.Join(", ", def.ValidValues)} — got \"{value}\"");
                }

                break;

            case SettingInputType.Number:
                var fractional = def.NumberStep?.Contains('.') == true;
                if (fractional)
                {
                    if (!double.TryParse(value, NumberStyles.Float, CultureInfo.InvariantCulture, out var f) || f < 0)
                    {
                        throw new SettingValidationException($"{key} must be a non-negative number — got \"{value}\"");
                    }
                }
                else if (!int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out var i) || i < 0)
                {
                    throw new SettingValidationException($"{key} must be a non-negative integer — got \"{value}\"");
                }

                break;

            case SettingInputType.ByteSize:
                if (!ByteSize.TryParse(value, out var n))
                {
                    throw new SettingValidationException($"{key}: invalid size \"{value}\"");
                }

                // A zero max upload would break all uploads; reject explicitly.
                if (key == "max_upload_size" && n == 0)
                {
                    throw new SettingValidationException($"{key} must be > 0");
                }

                break;

            case SettingInputType.Duration:
                try
                {
                    TokenLifetime.Parse(value);
                }
                catch (FormatException ex)
                {
                    throw new SettingValidationException($"{key}: {ex.Message}");
                }

                break;
        }
    }
}
