using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Identity;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.Server.Endpoints;

/// <summary>Cookie-authenticated account actions driven from the dashboard/account UI.</summary>
internal static class AccountEndpoints
{
    public static void MapAccountEndpoints(this IEndpointRouteBuilder endpoints)
    {
        var group = endpoints.MapGroup("/account").RequireAuthorization();
        group.MapPost("/apps/revoke", RevokeAppAsync);
    }

    // Disconnect a connected app by revoking one of the signed-in user's own tokens.
    private static async Task<IResult> RevokeAppAsync(
        HttpContext ctx, IAntiforgery antiforgery, TokenStore tokens, UserManager<ApplicationUser> users)
    {
        try
        {
            await antiforgery.ValidateRequestAsync(ctx);
        }
        catch (AntiforgeryValidationException)
        {
            return Results.BadRequest();
        }

        var user = await users.GetUserAsync(ctx.User);
        if (user is null)
        {
            return Results.Unauthorized();
        }

        var form = await ctx.Request.ReadFormAsync();
        var token = form["token"].ToString();
        if (!string.IsNullOrEmpty(token))
        {
            await tokens.RevokeForUserAsync(user.Id, token);
        }

        // Return to the page the disconnect was triggered from (dashboard or apps page).
        var returnUrl = form["returnUrl"].ToString();
        return Results.LocalRedirect(returnUrl.StartsWith('/') ? returnUrl : "/");
    }
}
