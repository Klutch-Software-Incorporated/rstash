using System.Security.Claims;

// GetOpenIddictServerRequest lives in the Microsoft.AspNetCore namespace, not an
// OpenIddict one.
using Microsoft.AspNetCore;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Identity;
using OpenIddict.Abstractions;
using OpenIddict.Server.AspNetCore;
using Rstash.Database;

namespace Rstash.Server.Identity;

/// <summary>
/// The embedded provider's endpoints. These sit at <c>/connect/*</c> and are the
/// human-identity counterpart to the remoteStorage app-authorization endpoints at
/// <c>/oauth/*</c>.
/// </summary>
internal static class ConnectEndpoints
{
    public static void MapConnectEndpoints(this IEndpointRouteBuilder endpoints)
    {
        endpoints.MapMethods(
            EmbeddedIdentityProvider.AuthorizeEndpoint,
            [HttpMethods.Get, HttpMethods.Post],
            AuthorizeAsync);

        endpoints.MapPost(EmbeddedIdentityProvider.TokenEndpoint, ExchangeAsync);
    }

    /// <summary>
    /// Turns an existing Identity cookie session into an authorization code.
    /// </summary>
    /// <remarks>
    /// This is the seam where the bundled provider meets local Identity: the provider
    /// does not own a password form of its own, it asks whoever is already signed in
    /// via the Identity cookie. If nobody is, it challenges that scheme, which lands
    /// the user on the existing /login page and returns here afterwards.
    /// </remarks>
    private static async Task<IResult> AuthorizeAsync(
        HttpContext context,
        UserManager<ApplicationUser> users)
    {
        var request = context.GetOpenIddictServerRequest()
            ?? throw new InvalidOperationException("not an OpenID Connect request");

        var result = await context.AuthenticateAsync(IdentityConstants.ApplicationScheme);
        if (!result.Succeeded)
        {
            return Results.Challenge(
                new AuthenticationProperties
                {
                    RedirectUri = context.Request.PathBase + context.Request.Path + context.Request.QueryString,
                },
                [IdentityConstants.ApplicationScheme]);
        }

        var user = await users.GetUserAsync(result.Principal)
            ?? throw new InvalidOperationException("authenticated principal has no user");

        var identity = new ClaimsIdentity(
            TokenValidationParameters.AuthenticationType,
            OpenIddictConstants.Claims.Name,
            OpenIddictConstants.Claims.Role);

        identity.SetClaim(OpenIddictConstants.Claims.Subject, user.Id.ToString())
            .SetClaim(OpenIddictConstants.Claims.PreferredUsername, user.UserName)
            .SetClaim(OpenIddictConstants.Claims.Email, user.Email);

        identity.SetScopes(request.GetScopes());

        // Without destinations a claim is minted but never emitted, which shows up
        // later as an ID token that inexplicably lacks the username.
        identity.SetDestinations(static claim => claim.Type switch
        {
            OpenIddictConstants.Claims.Subject
                or OpenIddictConstants.Claims.PreferredUsername
                or OpenIddictConstants.Claims.Email =>
                [OpenIddictConstants.Destinations.AccessToken, OpenIddictConstants.Destinations.IdentityToken],
            _ => [OpenIddictConstants.Destinations.AccessToken],
        });

        // Same reason as the token endpoint: sign in through the scheme's handler
        // directly rather than via Results.SignIn.
        await context.SignInAsync(
            OpenIddictServerAspNetCoreDefaults.AuthenticationScheme,
            new ClaimsPrincipal(identity));
        return Results.Empty;
    }

    /// <summary>Redeems the authorization code. Reached by the relying party's
    /// back-channel call, not by a browser.</summary>
    private static async Task<IResult> ExchangeAsync(HttpContext context)
    {
        var request = context.GetOpenIddictServerRequest()
            ?? throw new InvalidOperationException("not an OpenID Connect request");

        if (!request.IsAuthorizationCodeGrantType())
        {
            return Results.BadRequest(new { error = OpenIddictConstants.Errors.UnsupportedGrantType });
        }

        var result = await context.AuthenticateAsync(
            OpenIddictServerAspNetCoreDefaults.AuthenticationScheme);

        if (result.Principal is null)
        {
            return Results.BadRequest(new { error = OpenIddictConstants.Errors.InvalidGrant });
        }

        // SignInAsync directly, NOT Results.SignIn. The latter completes without error
        // and writes nothing at all — the token endpoint answers 200 with an empty body
        // and no content type, which looks like a serialization bug rather than a
        // plumbing one. Calling the scheme's handler ourselves is what makes OpenIddict
        // render the token response.
        await context.SignInAsync(
            OpenIddictServerAspNetCoreDefaults.AuthenticationScheme, result.Principal);
        return Results.Empty;
    }

    /// <summary>OpenIddict requires this exact authentication type on the identity it signs in.</summary>
    private static class TokenValidationParameters
    {
        public const string AuthenticationType = "OpenIddict.Server.AspNetCore";
    }
}
