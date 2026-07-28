using System.Buffers.Text;
using System.Security.Cryptography;
using System.Text;
using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Identity;
using Rstash.Database;
using Rstash.Model;
using Rstash.Services;
using Rstash.Services.Configuration;
using Rstash.Services.Entitlements;

namespace Rstash.Server.Endpoints;

/// <summary>
/// remoteStorage app-authorization OAuth: the consent POST, the PKCE token
/// exchange, and RFC 7009 revocation. (The consent UI is the Blazor page at
/// GET /oauth/authorize.)
/// </summary>
internal static class OAuthEndpoints
{
    public static void MapOAuthEndpoints(this IEndpointRouteBuilder endpoints)
    {
        // Distinct from the GET consent page (the Blazor @page "/oauth/authorize"):
        // a minimal-API POST on the same path would collide with the page endpoint.
        endpoints.MapPost("/oauth/authorize/decision", AuthorizeAsync);
        endpoints.MapPost("/oauth/token", TokenAsync).DisableAntiforgery().RequireCors("rstash-storage");
        endpoints.MapPost("/oauth/revoke", RevokeAsync).DisableAntiforgery().RequireCors("rstash-storage");
    }

    // POST /oauth/authorize — the consent form (same-origin, antiforgery-protected).
    private static async Task<IResult> AuthorizeAsync(
        HttpContext ctx,
        IAntiforgery antiforgery,
        UserManager<ApplicationUser> users,
        TokenStore tokens,
        SettingsService settings)
    {
        try
        {
            await antiforgery.ValidateRequestAsync(ctx);
        }
        catch (AntiforgeryValidationException)
        {
            return Results.BadRequest("invalid antiforgery token");
        }

        var form = await ctx.Request.ReadFormAsync();
        var redirectUri = form["redirect_uri"].ToString();
        var state = form["state"].ToString();
        var action = form["action"].ToString();
        var responseType = form["response_type"].ToString();
        var scopeStr = string.Join(' ', form["scope"].ToArray());
        var codeChallenge = form["code_challenge"].ToString();
        var codeChallengeMethod = form["code_challenge_method"].ToString();

        if (string.IsNullOrEmpty(redirectUri) || !Uri.TryCreate(redirectUri, UriKind.Absolute, out _))
        {
            return Results.BadRequest("invalid redirect_uri");
        }

        var isCodeFlow = responseType == "code";

        if (action == "deny")
        {
            var denyPairs = WithState("error=access_denied", state);
            return Results.Redirect(isCodeFlow ? WithQuery(redirectUri, denyPairs) : redirectUri + "#" + denyPairs);
        }

        var user = await users.GetUserAsync(ctx.User);
        if (user is null)
        {
            return Results.Unauthorized();
        }

        if (!Scope.TryParse(scopeStr, out _))
        {
            return Results.BadRequest("invalid scope");
        }

        var origin = ExtractOrigin(redirectUri);
        if (origin is null)
        {
            return Results.BadRequest("invalid redirect_uri");
        }

        if (isCodeFlow)
        {
            if (string.IsNullOrEmpty(codeChallenge) || codeChallengeMethod != "S256")
            {
                return Results.BadRequest("code flow requires code_challenge and code_challenge_method=S256");
            }

            var ac = await tokens.CreateCodeAsync(
                user.Id, origin, redirectUri, scopeStr, codeChallenge, codeChallengeMethod);
            return Results.Redirect(WithQuery(redirectUri, WithState($"code={Uri.EscapeDataString(ac.Code)}", state)));
        }

        // Implicit flow: token in the fragment.
        var lifetime = ParseLifetime(settings.Current.TokenLifetime);
        var token = await tokens.CreateAsync(user.Id, origin, scopeStr, lifetime);

        var fragment = $"access_token={Uri.EscapeDataString(token.Token)}&token_type=bearer";
        if (lifetime is { } span)
        {
            fragment += $"&expires_in={(long)span.TotalSeconds}";
        }

        fragment = WithState(fragment, state);
        return Results.Redirect(redirectUri + "#" + fragment);
    }

    // POST /oauth/token — authorization-code + PKCE exchange.
    private static async Task<IResult> TokenAsync(
        HttpContext ctx, UserManager<ApplicationUser> users, TokenStore tokens, SettingsService settings,
        IEntitlementSource entitlements)
    {
        ctx.Response.Headers.CacheControl = "no-store";

        var form = await ctx.Request.ReadFormAsync();
        if (form["grant_type"].ToString() != "authorization_code")
        {
            return TokenError("unsupported_grant_type", "supported grant types: authorization_code", 400);
        }

        var code = form["code"].ToString();
        var codeVerifier = form["code_verifier"].ToString();
        var redirectUri = form["redirect_uri"].ToString();

        if (code.Length == 0 || codeVerifier.Length == 0 || redirectUri.Length == 0)
        {
            return TokenError("invalid_request", "code, code_verifier, and redirect_uri are required", 400);
        }

        var ac = await tokens.GetValidCodeAsync(code);
        if (ac is null)
        {
            return TokenError("invalid_grant", "authorization code is invalid, expired, or already used", 400);
        }

        if (ac.RedirectUri != redirectUri)
        {
            return TokenError("invalid_grant", "redirect_uri does not match", 400);
        }

        if (!VerifyPkce(codeVerifier, ac.CodeChallenge))
        {
            return TokenError("invalid_grant", "code_verifier does not match code_challenge", 400);
        }

        var user = await users.FindByIdAsync(ac.UserId.ToString(System.Globalization.CultureInfo.InvariantCulture));
        if (user is null)
        {
            return TokenError("invalid_grant", "user account is disabled", 400);
        }

        // Minting an app token is a good moment to re-check standing: the entitlement
        // source folds local approval together with the provider's kill switch.
        if ((await entitlements.ResolveAsync(user.Id)).Disabled)
        {
            return TokenError("invalid_grant", "user account is disabled", 400);
        }

        await tokens.UseCodeAsync(code);

        var lifetime = ParseLifetime(settings.Current.TokenLifetime);
        var token = await tokens.CreateAsync(ac.UserId, ac.ClientId, ac.Scopes, lifetime);

        var response = new Dictionary<string, object>
        {
            ["access_token"] = token.Token,
            ["token_type"] = "bearer",
            ["scope"] = ac.Scopes,
        };
        if (lifetime is { } span)
        {
            response["expires_in"] = (long)span.TotalSeconds;
        }

        return Results.Json(response);
    }

    // POST /oauth/revoke — RFC 7009 (always 200).
    private static async Task<IResult> RevokeAsync(HttpContext ctx, TokenStore tokens)
    {
        var form = await ctx.Request.ReadFormAsync();
        var token = form["token"].ToString();
        if (token.Length == 0)
        {
            return Results.BadRequest("token parameter is required");
        }

        await tokens.RevokeAsync(token);
        return Results.Ok();
    }

    private static bool VerifyPkce(string verifier, string challenge)
    {
        var hash = SHA256.HashData(Encoding.ASCII.GetBytes(verifier));
        return Base64Url.EncodeToString(hash) == challenge;
    }

    private static TimeSpan? ParseLifetime(string value)
    {
        var span = TokenLifetime.Parse(value);
        return span == TimeSpan.Zero ? null : span;
    }

    private static string? ExtractOrigin(string rawUrl) =>
        Uri.TryCreate(rawUrl, UriKind.Absolute, out var uri)
        && !string.IsNullOrEmpty(uri.Scheme) && !string.IsNullOrEmpty(uri.Host)
            ? $"{uri.Scheme}://{uri.Authority}"
            : null;

    private static string WithState(string pairs, string state) =>
        string.IsNullOrEmpty(state) ? pairs : $"{pairs}&state={Uri.EscapeDataString(state)}";

    private static string WithQuery(string url, string rawPairs) =>
        url + (url.Contains('?', StringComparison.Ordinal) ? '&' : '?') + rawPairs;

    private static IResult TokenError(string error, string description, int statusCode) =>
        Results.Json(new { error, error_description = description }, statusCode: statusCode);
}
