using System.Text.RegularExpressions;

namespace Rstash.IntegrationTests;

internal static partial class FormHelpers
{
    /// <summary>Pulls the antiforgery token out of a server-rendered form.</summary>
    public static string AntiforgeryToken(string html)
    {
        var match = TokenRegex().Match(html);
        if (!match.Success)
        {
            throw new InvalidOperationException("antiforgery token not found in form HTML");
        }

        return match.Groups[1].Value;
    }

    [GeneratedRegex("name=\"__RequestVerificationToken\"[^>]*value=\"([^\"]+)\"")]
    private static partial Regex TokenRegex();
}
