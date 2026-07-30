using System.Security.Claims;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Identity;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;
using OpenIddict.Abstractions;
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

                // Query, not the handler's default of form_post. form_post returns the
                // authorization response as a self-submitting HTML form, which arrives
                // back as a *cross-site* POST — so the handler marks its correlation
                // cookie SameSite=None, and that requires Secure. On plain HTTP the
                // cookie is therefore dropped and the callback fails with "Correlation
                // failed", which is precisely the two configurations rstash actually
                // runs in without app-level TLS: local development, and behind a proxy
                // that terminates TLS upstream.
                //
                // Query mode makes the callback an ordinary same-site redirect, so a
                // Lax cookie survives. Nothing is weakened: this is authorization code
                // flow, so the only thing in the URL is a single-use, PKCE-bound code.
                options.ResponseMode = OpenIdConnectResponseMode.Query;

                // The handler's own cookies have to follow response mode down. Both the
                // correlation and nonce cookies default to SameSite=None, which is only
                // legal alongside Secure — so their SecurePolicy defaults to Always, and
                // on plain HTTP the browser (and HttpClient) simply refuses to send them
                // back. The callback then fails with "Correlation failed", having never
                // received the cookie it is trying to match.
                //
                // Those defaults are built for form_post, where the response really does
                // arrive cross-site. In query mode the callback is a same-site top-level
                // GET, so Lax is sufficient, and Secure can track the origin rstash has
                // actually been told it serves — the same signal RequireHttpsMetadata and
                // the provider's transport-security requirement key off.
                var secureCookies = baseUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase)
                    ? CookieSecurePolicy.Always
                    : CookieSecurePolicy.SameAsRequest;

                options.CorrelationCookie.SameSite = SameSiteMode.Lax;
                options.CorrelationCookie.SecurePolicy = secureCookies;
                options.NonceCookie.SameSite = SameSiteMode.Lax;
                options.NonceCookie.SecurePolicy = secureCookies;

                // The provider is a public client here, so there is no secret to send.
                options.ClientSecret = null;

                options.Scope.Clear();
                options.Scope.Add("openid");
                options.Scope.Add("profile");
                options.Scope.Add("email");

                options.GetClaimsFromUserInfoEndpoint = true;
                options.SaveTokens = false;

                // Keep the claim types the provider actually sent rather than rewriting
                // them to the legacy SOAP URIs, so provisioning can read "sub" and
                // "preferred_username" as written in the spec.
                options.MapInboundClaims = false;

                // The consequence of that, though, is that nothing populates
                // Identity.Name, which the layout and the OAuth consent screen display.
                options.TokenValidationParameters.NameClaimType =
                    OpenIddictConstants.Claims.PreferredUsername;

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

                options.Events.OnTokenValidated = async context =>
                {
                    AddLocalUserIdClaim(context.Principal);
                    await JitProvisioning.OnTokenValidatedAsync(context);
                };
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

    /// <summary>
    /// Republishes the provider's <c>sub</c> as the claim type ASP.NET Core Identity
    /// looks the current user up by.
    /// </summary>
    /// <remarks>
    /// <para>
    /// Every authenticated surface in rstash — the file browser, the account and admin
    /// pages, OAuth consent — resolves the signed-in person through
    /// <see cref="UserManager{TUser}.GetUserAsync"/>, which reads
    /// <see cref="ClaimTypes.NameIdentifier"/>. Nothing emits that claim here:
    /// <c>MapInboundClaims</c> is off, so the principal carries <c>sub</c> verbatim.
    /// Without this the lookup returns null on every request and the whole signed-in
    /// application renders empty — it fails closed, so nothing is exposed, but nothing
    /// works either.
    /// </para>
    /// <para>
    /// The mapping is sound because in embedded mode the provider mints <c>sub</c> from
    /// the local <c>ApplicationUser.Id</c>. Under an external provider it will not be:
    /// <c>sub</c> becomes the control plane's own identifier, and resolution has to go
    /// through <c>StorageUser.Subject</c> instead — which is why provisioning already
    /// records it there.
    /// </para>
    /// </remarks>
    private static void AddLocalUserIdClaim(ClaimsPrincipal? principal)
    {
        if (principal?.Identity is not ClaimsIdentity identity
            || principal.FindFirst(ClaimTypes.NameIdentifier) is not null)
        {
            return;
        }

        if (principal.FindFirst(OpenIddictConstants.Claims.Subject)?.Value is { Length: > 0 } subject)
        {
            identity.AddClaim(new Claim(ClaimTypes.NameIdentifier, subject));
        }
    }
}
