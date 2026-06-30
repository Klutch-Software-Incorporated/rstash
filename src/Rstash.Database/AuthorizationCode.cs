namespace Rstash.Database;

/// <summary>
/// A one-time OAuth authorization code (authorization-code + PKCE flow). Issued
/// at consent, exchanged once at the token endpoint.
/// </summary>
public class AuthorizationCode
{
    public string Code { get; set; } = "";

    public long UserId { get; set; }

    public string ClientId { get; set; } = "";

    public string RedirectUri { get; set; } = "";

    public string Scopes { get; set; } = "";

    public string CodeChallenge { get; set; } = "";

    public string CodeChallengeMethod { get; set; } = "";

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset ExpiresAt { get; set; }

    public bool Used { get; set; }
}
