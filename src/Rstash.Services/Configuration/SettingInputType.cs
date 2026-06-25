namespace Rstash.Services.Configuration;

/// <summary>How a setting renders in the admin UI (and how it is validated).</summary>
public enum SettingInputType
{
    Text,
    Number,
    Select,
    ByteSize,
    Duration,
    TextArea,
}
