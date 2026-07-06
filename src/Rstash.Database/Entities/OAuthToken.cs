namespace Rstash.Database;

/// <summary>
/// A remoteStorage app-authorization access token (distinct from user-identity
/// OIDC). Grants a client scoped access to one user's storage.
/// </summary>
public class OAuthToken
{
    public string Token { get; set; } = "";

    public long UserId { get; set; }

    public string ClientId { get; set; } = "";

    /// <summary>Space-separated remoteStorage scopes (e.g. "documents:rw").</summary>
    public string Scopes { get; set; } = "";

    public DateTimeOffset CreatedAt { get; set; }

    /// <summary>Null = no expiry.</summary>
    public DateTimeOffset? ExpiresAt { get; set; }
}
