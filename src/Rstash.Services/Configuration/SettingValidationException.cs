namespace Rstash.Services.Configuration;

/// <summary>Thrown when a setting value fails validation before being persisted.</summary>
public sealed class SettingValidationException(string message) : Exception(message);
