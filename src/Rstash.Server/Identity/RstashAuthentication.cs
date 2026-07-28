using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Identity;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;
using Rstash.Database;

namespace Rstash.Server.Identity;

/// <summary>
/// Wires rstash as an OpenID Connect relying party, with the provider either bundled
/// or (later) external.
/// </summary>
/// <remarks>
/// <para>
/// <b>Two cookies, deliberately.</b> The Identity cookie is the *provider's* session:
/// it is what the password form sets and what <c>/connect/authorize</c> reads to
/// decide who is being authorized. The application cookie is the *relying party's*
/// session, set only after a completed code exchange.
/// </para>
/// <para>
/// They cannot be the same cookie. If the application challenged the scheme that
/// <c>/connect/authorize</c> itself authenticates against, the challenge would send
/// the user to the authorize endpoint, which would find no session and challenge
/// again — forever. Splitting them is what terminates the loop: the application
/// challenges OpenID Connect, and the provider challenges Identity, whose login path
/// is the password form.
/// </para>
/// <para>
/// The cost is that a logout has to clear both, and that embedded mode performs a
/// redirect round-trip it could technically skip. What it buys is a single
/// claims-to-<see cref="StorageUser"/> path shared with external mode, so moving the
/// provider out is a configuration change rather than a second implementation.
/// </para>
/// </remarks>
internal static class RstashAuthentication
{
    /// <summary>The relying party's session cookie — the application's own sign-in.</summary>
    public const string ApplicationScheme = "rstash.session";

    public static IServiceCollection AddRstashAuthentication(
        this IServiceCollection services, string baseUrl)
    {
        services.AddAuthentication(options =>
            {
                options.DefaultScheme = ApplicationScheme;
                options.DefaultSignInScheme = ApplicationScheme;

                // An unauthenticated request starts the OpenID Connect flow rather than
                // going straight to a password form; the provider decides how the human
                // proves who they are.
                options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
            })
            .AddCookie(ApplicationScheme, options =>
            {
                options.Cookie.Name = ApplicationScheme;
                options.Cookie.HttpOnly = true;
                options.Cookie.SameSite = SameSiteMode.Lax;
                options.SlidingExpiration = true;

                // Never a password form: this scheme's challenge is OpenID Connect.
                options.LoginPath = "/login";
                options.AccessDeniedPath = "/login";
                options.ReturnUrlParameter = "redirect";
            })
            .AddOpenIdConnect(options =>
            {
                // Against ourselves in embedded mode; in external mode this becomes the
                // control plane and nothing else here changes.
                options.Authority = baseUrl;
                options.ClientId = EmbeddedIdentityProvider.ClientId;
                options.CallbackPath = EmbeddedIdentityProvider.CallbackPath;
                options.SignInScheme = ApplicationScheme;

                options.ResponseType = OpenIdConnectResponseType.Code;
                options.UsePkce = true;

                // The provider is a public client here, so there is no secret to send.
                options.ClientSecret = null;

                options.Scope.Clear();
                options.Scope.Add("openid");
                options.Scope.Add("profile");
                options.Scope.Add("email");

                options.GetClaimsFromUserInfoEndpoint = true;
                options.SaveTokens = false;
                options.MapInboundClaims = false;

                // Local development and proxy-terminated TLS both leave the app on
                // plaintext HTTP; requiring https metadata would make discovery fail.
                options.RequireHttpsMetadata =
                    baseUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase);

                // The handler derives redirect_uri from the incoming request, which is
                // the same trap RSTASH_BASE_URL exists to avoid: behind a
                // TLS-terminating proxy the app believes it lives at
                // http://localhost:8080, so it would send a redirect_uri that does not
                // match the registered one and the provider would reject the request.
                // Pin it on both legs — the value is recomputed for the token exchange
                // rather than carried over from the authorize call, and the two must
                // agree or the code redemption fails.
                var redirectUri = $"{baseUrl}{EmbeddedIdentityProvider.CallbackPath}";

                options.Events.OnRedirectToIdentityProvider = context =>
                {
                    context.ProtocolMessage.RedirectUri = redirectUri;
                    return Task.CompletedTask;
                };

                options.Events.OnAuthorizationCodeReceived = context =>
                {
                    if (context.TokenEndpointRequest is { } request)
                    {
                        request.RedirectUri = redirectUri;
                    }

                    return Task.CompletedTask;
                };

                options.Events.OnTokenValidated = JitProvisioning.OnTokenValidatedAsync;
            });

        // Registers the Identity cookie schemes without disturbing the defaults set
        // above: this is the provider's session, not the application's.
        services.AddAuthentication().AddIdentityCookies();

        services.ConfigureApplicationCookie(options =>
        {
            options.LoginPath = "/login";
            options.AccessDeniedPath = "/login";
            options.ReturnUrlParameter = "redirect";
        });

        return services;
    }
}
