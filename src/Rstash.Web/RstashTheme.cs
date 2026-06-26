using MudBlazor;

namespace Rstash.Web;

/// <summary>
/// The rstash brand theme. A single shared <see cref="MudTheme"/> consumed by the layout's
/// <c>MudThemeProvider</c>; light/dark are selected at render time from a cookie preference.
/// Brand: warm gold (<c>#d4a840</c>) and deep gold (<c>#8a6d00</c>) on an off-white canvas.
/// </summary>
public static class RstashTheme
{
    /// <summary>Cookie that stores the user's light/dark preference (<c>"dark"</c> or <c>"light"</c>).</summary>
    public const string ThemeCookie = "rstash_theme";

    public static readonly MudTheme Theme = new()
    {
        PaletteLight = new PaletteLight
        {
            Primary = "#D4A840",
            PrimaryContrastText = "#2A2007",
            Secondary = "#8A6D00",
            SecondaryContrastText = "#FFFFFF",
            AppbarBackground = "#FAFAF8",
            AppbarText = "#2A2620",
            Background = "#FAFAF8",
            BackgroundGray = "#F2F1EC",
            Surface = "#FFFFFF",
            DrawerBackground = "#FFFFFF",
            TextPrimary = "#2A2620",
            TextSecondary = "#7A7468",
            ActionDefault = "#7A7468",
            LinesDefault = "#E9E5DB",
            LinesInputs = "#E9E5DB",
            TableLines = "#E9E5DB",
            Divider = "#E9E5DB",
            Success = "#2E9E6B",
            Warning = "#C9810B",
            Error = "#C0392B",
        },
        PaletteDark = new PaletteDark
        {
            Primary = "#D4A840",
            PrimaryContrastText = "#2A2007",
            Secondary = "#D8B85F",
            SecondaryContrastText = "#2A2007",
            AppbarBackground = "#211F1A",
            AppbarText = "#ECE7DA",
            Background = "#1A1814",
            BackgroundGray = "#13110E",
            Surface = "#211F1A",
            DrawerBackground = "#211F1A",
            TextPrimary = "#ECE7DA",
            TextSecondary = "#A89F8D",
            ActionDefault = "#A89F8D",
            LinesDefault = "#3A352C",
            LinesInputs = "#3A352C",
            TableLines = "#3A352C",
            Divider = "#3A352C",
            Success = "#3FBE82",
            Warning = "#E0A93B",
            Error = "#E5707A",
        },
        Typography = new Typography
        {
            Default = new DefaultTypography
            {
                FontFamily = ["Roboto", "system-ui", "sans-serif"],
                FontSize = "0.9375rem",
                LineHeight = "1.5",
            },
        },
        LayoutProperties = new LayoutProperties
        {
            DefaultBorderRadius = "10px",
        },
    };
}
