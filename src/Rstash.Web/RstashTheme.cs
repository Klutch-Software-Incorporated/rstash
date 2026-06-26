using MudBlazor;

namespace Rstash.Web;

/// <summary>
/// The rstash brand theme. A single shared <see cref="MudTheme"/> consumed by the layout's
/// <c>MudThemeProvider</c>; light/dark are selected at render time from a cookie preference.
/// </summary>
public static class RstashTheme
{
    /// <summary>Cookie that stores the user's light/dark preference (<c>"dark"</c> or <c>"light"</c>).</summary>
    public const string ThemeCookie = "rstash_theme";

    public static readonly MudTheme Theme = new()
    {
        PaletteLight = new PaletteLight
        {
            Primary = "#5B61E8",
            Secondary = "#16B8A6",
            AppbarBackground = "#5B61E8",
            AppbarText = "#FFFFFF",
            Background = "#F6F7FB",
            BackgroundGray = "#EEF0F6",
            Surface = "#FFFFFF",
            DrawerBackground = "#FFFFFF",
            Success = "#2E9E6B",
            Warning = "#C9810B",
            Error = "#D64550",
            TextPrimary = "#1D2433",
            TextSecondary = "#5A6478",
        },
        PaletteDark = new PaletteDark
        {
            Primary = "#8B90F5",
            Secondary = "#2BD4C3",
            AppbarBackground = "#1A1C23",
            AppbarText = "#ECEDF4",
            Background = "#121319",
            BackgroundGray = "#0D0E13",
            Surface = "#1A1C23",
            DrawerBackground = "#1A1C23",
            Success = "#3FBE82",
            Warning = "#E0A93B",
            Error = "#E5707A",
            TextPrimary = "#ECEDF4",
            TextSecondary = "#A4ABBD",
        },
        LayoutProperties = new LayoutProperties
        {
            DefaultBorderRadius = "10px",
        },
    };
}
