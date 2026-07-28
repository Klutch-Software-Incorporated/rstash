namespace Rstash.Database;

/// <summary>
/// Persisted key material for the embedded OpenID Connect provider, one row per
/// purpose. See the <c>AddOidcKeys</c> migration for why these outlive the process.
/// </summary>
public class OidcKey
{
    /// <summary>"signing" or "encryption". Also the primary key, so the row cannot be
    /// duplicated and two instances racing on first boot cannot both win.</summary>
    public string Purpose { get; set; } = "";

    /// <summary>
    /// Data-Protection–protected RSA private key. The protection is not a boundary
    /// against a full database compromise — the Data Protection ring lives in the same
    /// database — but it keeps raw private key material out of backups, dumps, and any
    /// admin view that ever learns to render this table.
    /// </summary>
    public string KeyMaterial { get; set; } = "";

    public DateTimeOffset CreatedAt { get; set; }
}
