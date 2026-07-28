using Microsoft.EntityFrameworkCore;
using OpenIddict.Abstractions;
using Rstash.Database;

namespace Rstash.Server.Identity;

/// <summary>
/// Wires the bundled OpenID Connect provider — the one self-hosters get without
/// configuring anything external.
/// </summary>
/// <remarks>
/// This is deliberately separate from the remoteStorage app-authorization server at
/// <c>/oauth/*</c>. That one authorizes third-party *apps* against the spec's own
/// OAuth profile and issues opaque bearer tokens; this one authorizes *humans* and
/// issues ID tokens. They live at different paths, use different stores, and must
/// not be merged.
/// </remarks>
internal static class EmbeddedIdentityProvider
{
    /// <summary>The client rstash registers for itself as a relying party.</summary>
    public const string ClientId = "rstash-web";

    public const string AuthorizeEndpoint = "/connect/authorize";
    public const string TokenEndpoint = "/connect/token";
    public const string UserInfoEndpoint = "/connect/userinfo";
    public const string CallbackPath = "/signin-oidc";

    public static IServiceCollection AddEmbeddedIdentityProvider(
        this IServiceCollection services, string baseUrl)
    {
        services.AddOpenIddict()
            .AddCore(options => options
                .UseEntityFrameworkCore()
                .UseDbContext<RstashDbContext>())
            .AddServer(options =>
            {
                // Pinned rather than inferred from the request: the issuer must be
                // byte-identical in the discovery document and every token, and behind
                // a TLS-terminating proxy the request would report the wrong origin.
                options.SetIssuer(baseUrl);

                options.SetAuthorizationEndpointUris(AuthorizeEndpoint)
                    .SetTokenEndpointUris(TokenEndpoint)
                    .SetUserInfoEndpointUris(UserInfoEndpoint);

                // Authorization code + PKCE only. No implicit, no password grant: the
                // former is deprecated and the latter would defeat the point of putting
                // credentials behind a provider.
                options.AllowAuthorizationCodeFlow()
                    .RequireProofKeyForCodeExchange();

                options.RegisterScopes(
                    OpenIddictConstants.Scopes.OpenId,
                    OpenIddictConstants.Scopes.Profile,
                    OpenIddictConstants.Scopes.Email);

                // SPIKE ONLY — ephemeral keys are regenerated on every restart, which
                // breaks logins that are mid-flight across it. Established sessions
                // survive, because login yields an ordinary auth cookie, so this is not
                // as severe as it sounds. Still must become DB-persisted, rotating keys
                // before this ships.
                options.AddEphemeralEncryptionKey()
                    .AddEphemeralSigningKey();

                // Passthrough on the two endpoints that need rstash's own logic. UserInfo
                // is deliberately left to OpenIddict, which answers it from the access
                // token's claims — there is nothing for rstash to add, and enabling
                // passthrough without mapping a route silently 404s the endpoint.
                var aspNetCore = options.UseAspNetCore()
                    .EnableAuthorizationEndpointPassthrough()
                    .EnableTokenEndpointPassthrough();

                // OpenIddict refuses plaintext HTTP by default — every endpoint,
                // including discovery, answers 400. That default is right for a public
                // issuer, but it is wrong for the two ways rstash actually runs without
                // TLS at the app: local development, and behind a proxy that terminates
                // TLS upstream (where the app legitimately sees http). Keying this off
                // the configured base URL means an operator who declares an https origin
                // keeps the protection, and only one who has declared http loses it.
                if (baseUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase))
                {
                    aspNetCore.DisableTransportSecurityRequirement();
                }
            })
            .AddValidation(options =>
            {
                options.UseLocalServer();
                options.UseAspNetCore();
            });

        return services;
    }

    /// <summary>
    /// Registers rstash's own client row on startup, idempotently. In embedded mode
    /// the provider and the relying party are the same deployment, so nobody is
    /// around to configure this by hand.
    /// </summary>
    public static async Task EnsureClientRegisteredAsync(IServiceProvider services, string baseUrl)
    {
        await using var scope = services.CreateAsyncScope();
        var manager = scope.ServiceProvider.GetRequiredService<IOpenIddictApplicationManager>();

        if (await manager.FindByClientIdAsync(ClientId) is not null)
        {
            return;
        }

        await manager.CreateAsync(new OpenIddictApplicationDescriptor
        {
            ClientId = ClientId,
            ClientSecret = null,
            ClientType = OpenIddictConstants.ClientTypes.Public,
            ConsentType = OpenIddictConstants.ConsentTypes.Implicit,
            DisplayName = "rstash",
            RedirectUris = { new Uri($"{baseUrl}{CallbackPath}") },
            Permissions =
            {
                OpenIddictConstants.Permissions.Endpoints.Authorization,
                OpenIddictConstants.Permissions.Endpoints.Token,
                OpenIddictConstants.Permissions.GrantTypes.AuthorizationCode,
                OpenIddictConstants.Permissions.ResponseTypes.Code,
                OpenIddictConstants.Permissions.Scopes.Email,
                OpenIddictConstants.Permissions.Scopes.Profile,
            },
            Requirements =
            {
                OpenIddictConstants.Requirements.Features.ProofKeyForCodeExchange,
            },
        });
    }
}
