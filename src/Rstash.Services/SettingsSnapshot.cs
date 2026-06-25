using System.Globalization;
using Rstash.Services.Configuration;

namespace Rstash.Services;

/// <summary>
/// The resolved runtime settings — DB overrides merged on top of registry
/// defaults — as a typed, immutable snapshot. Lock-free to read.
/// </summary>
public sealed record SettingsSnapshot
{
    public required string MetricsMode { get; init; }
    public required string RegistrationMode { get; init; }
    public required string LogLevel { get; init; }
    public required double RateLimitRate { get; init; }
    public required int RateLimitBurst { get; init; }
    public required double UserRateLimitRate { get; init; }
    public required int UserRateLimitBurst { get; init; }
    public required long TotalStorageLimit { get; init; }
    public required long DefaultUserStorageLimit { get; init; }
    public required long MaxUploadSize { get; init; }
    public required long TotalEgressLimit { get; init; }
    public required long DefaultUserEgressLimit { get; init; }
    public required string TokenLifetime { get; init; }
    public required string RefreshTokens { get; init; }
    public required string RefreshTokenLifetime { get; init; }
    public required string SiteName { get; init; }
    public required string HomeSubtitle { get; init; }
    public required string BlockedMimeTypes { get; init; }
    public required string TosMode { get; init; }
    public required string TosContent { get; init; }
    public required string PrivacyMode { get; init; }
    public required string PrivacyContent { get; init; }
    public required string CookieDomain { get; init; }
    public required string RegistrationExternalUrl { get; init; }
    public required string ExternalAccountUrl { get; init; }
    public required IReadOnlyList<CustomLink> CustomLinks { get; init; }
    public required string DisabledAccountMessage { get; init; }
    public required int AuditRetentionDays { get; init; }
    public required string LogClientIps { get; init; }

    /// <summary>
    /// Builds a snapshot from optional DB overrides, falling back to each
    /// setting's registry default. Malformed override values fall back to the
    /// default (writes are validated, so this only guards against drift).
    /// </summary>
    public static SettingsSnapshot Resolve(IReadOnlyDictionary<string, string>? overrides)
    {
        string Val(string key) =>
            overrides is not null && overrides.TryGetValue(key, out var v)
                ? v
                : SettingDefinitions.ByKey(key)?.Default ?? "";

        long Bytes(string key) =>
            ByteSize.TryParse(Val(key), out var n)
                ? n
                : ByteSize.TryParse(SettingDefinitions.ByKey(key)!.Default, out var fallback) ? fallback : 0;

        double Rate(string key) =>
            double.TryParse(Val(key), NumberStyles.Float, CultureInfo.InvariantCulture, out var f) && f >= 0
                ? f
                : double.Parse(SettingDefinitions.ByKey(key)!.Default, CultureInfo.InvariantCulture);

        int Count(string key) =>
            int.TryParse(Val(key), NumberStyles.Integer, CultureInfo.InvariantCulture, out var i) && i >= 0
                ? i
                : int.Parse(SettingDefinitions.ByKey(key)!.Default, CultureInfo.InvariantCulture);

        IReadOnlyList<CustomLink> links;
        try
        {
            links = Configuration.CustomLinks.Parse(Val("custom_links"));
        }
        catch (SettingValidationException)
        {
            links = [];
        }

        return new SettingsSnapshot
        {
            MetricsMode = Val("metrics_mode"),
            RegistrationMode = Val("registration_mode"),
            LogLevel = Val("log_level"),
            RateLimitRate = Rate("rate_limit_rate"),
            RateLimitBurst = Count("rate_limit_burst"),
            UserRateLimitRate = Rate("user_rate_limit_rate"),
            UserRateLimitBurst = Count("user_rate_limit_burst"),
            TotalStorageLimit = Bytes("total_storage_limit"),
            DefaultUserStorageLimit = Bytes("default_user_storage_limit"),
            MaxUploadSize = Bytes("max_upload_size"),
            TotalEgressLimit = Bytes("total_egress_limit"),
            DefaultUserEgressLimit = Bytes("default_user_egress_limit"),
            TokenLifetime = Val("token_lifetime"),
            RefreshTokens = Val("refresh_tokens"),
            RefreshTokenLifetime = Val("refresh_token_lifetime"),
            SiteName = Val("site_name"),
            HomeSubtitle = Val("home_subtitle"),
            BlockedMimeTypes = Val("blocked_mime_types"),
            TosMode = Val("tos_mode"),
            TosContent = Val("tos_content"),
            PrivacyMode = Val("privacy_mode"),
            PrivacyContent = Val("privacy_content"),
            CookieDomain = Val("cookie_domain"),
            RegistrationExternalUrl = Val("registration_external_url"),
            ExternalAccountUrl = Val("external_account_url"),
            CustomLinks = links,
            DisabledAccountMessage = Val("disabled_account_message"),
            AuditRetentionDays = Count("audit_retention_days"),
            LogClientIps = Val("log_client_ips"),
        };
    }
}
