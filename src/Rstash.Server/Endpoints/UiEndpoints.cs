using Rstash.Web;

namespace Rstash.Server.Endpoints;

/// <summary>Small cookie-backed UI preferences (no auth; cosmetic only).</summary>
internal static class UiEndpoints
{
    public static void MapUiEndpoints(this IEndpointRouteBuilder endpoints)
    {
        // Persist the light/dark preference in a cookie, then bounce back to the page the user
        // was on. The layout reads this cookie server-side to drive MudThemeProvider.IsDarkMode.
        endpoints.MapGet("/ui/theme", (HttpContext context, string? mode, string? @return) =>
        {
            var dark = string.Equals(mode, "dark", StringComparison.OrdinalIgnoreCase);
            context.Response.Cookies.Append(RstashTheme.ThemeCookie, dark ? "dark" : "light", new CookieOptions
            {
                Path = "/",
                MaxAge = TimeSpan.FromDays(365),
                SameSite = SameSiteMode.Lax,
                IsEssential = true,
            });

            // Only ever redirect to a local path — never honor an off-site return target.
            var target = @return is { Length: > 0 } value && value.StartsWith('/') ? value : "/";
            return Results.LocalRedirect(target);
        });
    }
}
