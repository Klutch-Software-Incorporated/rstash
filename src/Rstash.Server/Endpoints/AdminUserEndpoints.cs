using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Identity;
using Rstash.Database;
using Rstash.Services.Configuration;

namespace Rstash.Server.Endpoints;

/// <summary>Admin user-management mutations (cookie-authed, IsAdmin-gated).</summary>
internal static class AdminUserEndpoints
{
    public static void MapAdminUserEndpoints(this IEndpointRouteBuilder endpoints)
    {
        var group = endpoints.MapGroup("/admin/users").RequireAuthorization();
        group.MapPost("/approve", ApproveAsync);
        group.MapPost("/toggle-disabled", ToggleDisabledAsync);
        group.MapPost("/delete", DeleteAsync);
        group.MapPost("/quota", QuotaAsync);
    }

    private static async Task<IResult> ApproveAsync(HttpContext ctx, IAntiforgery af, UserManager<ApplicationUser> users)
    {
        var (ok, _, target) = await ResolveAsync(ctx, af, users);
        if (!ok)
        {
            return Results.Forbid();
        }

        if (target is not null)
        {
            target.Approved = true;
            await users.UpdateAsync(target);
        }

        return Results.Redirect("/admin/users");
    }

    // Admins — including the acting admin — are shielded from disable/delete so the
    // server can never be locked out of all administrative access.
    private static bool IsProtected(ApplicationUser actor, ApplicationUser target) =>
        target.IsAdmin || target.Id == actor.Id;

    private static async Task<IResult> ToggleDisabledAsync(HttpContext ctx, IAntiforgery af, UserManager<ApplicationUser> users)
    {
        var (ok, actor, target) = await ResolveAsync(ctx, af, users);
        if (!ok)
        {
            return Results.Forbid();
        }

        // Admins (incl. yourself) must retain access — never disable them.
        if (target is not null && !IsProtected(actor, target))
        {
            target.Disabled = !target.Disabled;
            await users.UpdateAsync(target);
        }

        return Results.Redirect("/admin/users");
    }

    private static async Task<IResult> DeleteAsync(HttpContext ctx, IAntiforgery af, UserManager<ApplicationUser> users)
    {
        var (ok, actor, target) = await ResolveAsync(ctx, af, users);
        if (!ok)
        {
            return Results.Forbid();
        }

        // Admins (incl. yourself) must retain access — never delete them.
        if (target is not null && !IsProtected(actor, target))
        {
            await users.DeleteAsync(target);
        }

        return Results.Redirect("/admin/users");
    }

    private static async Task<IResult> QuotaAsync(HttpContext ctx, IAntiforgery af, UserManager<ApplicationUser> users)
    {
        var (ok, _, target) = await ResolveAsync(ctx, af, users);
        if (!ok)
        {
            return Results.Forbid();
        }

        if (target is not null)
        {
            var raw = ctx.Request.Form["storageQuota"].ToString();
            target.StorageQuota = ByteSize.TryParse(raw, out var bytes) ? bytes : 0;
            await users.UpdateAsync(target);
        }

        return Results.Redirect("/admin/users");
    }

    private static async Task<(bool Ok, ApplicationUser Actor, ApplicationUser? Target)> ResolveAsync(
        HttpContext ctx, IAntiforgery af, UserManager<ApplicationUser> users)
    {
        try
        {
            await af.ValidateRequestAsync(ctx);
        }
        catch (AntiforgeryValidationException)
        {
            return (false, null!, null);
        }

        var actor = await users.GetUserAsync(ctx.User);
        if (actor is null || !actor.IsAdmin)
        {
            return (false, null!, null);
        }

        var id = (await ctx.Request.ReadFormAsync())["id"].ToString();
        return (true, actor, await users.FindByIdAsync(id));
    }
}
