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

    /// <summary>
    /// Signs in the way a person does: request a protected page, land on whatever form
    /// the challenge chain produces, and submit it.
    /// </summary>
    /// <remarks>
    /// The form posts back to itself, and its action carries the return URL that
    /// resumes the interrupted authorization — so the action has to be read off the
    /// page rather than assumed to be "/login", or the round-trip ends at the form
    /// instead of at the page that was asked for. Requires a redirect-following client.
    /// </remarks>
    public static async Task<HttpResponseMessage> SignInAsync(
        HttpClient client, string username, string password, string startPath = "/")
    {
        var html = await (await client.GetAsync(startPath)).Content.ReadAsStringAsync();
        var action = System.Net.WebUtility.HtmlDecode(FormActionRegex().Match(html).Groups[1].Value);

        return await client.PostAsync(
            string.IsNullOrEmpty(action) ? startPath : action,
            new FormUrlEncodedContent(new Dictionary<string, string>
            {
                ["_handler"] = "login",
                ["__RequestVerificationToken"] = AntiforgeryToken(html),
                ["Input.Username"] = username,
                ["Input.Password"] = password,
            }));
    }

    [GeneratedRegex("name=\"__RequestVerificationToken\"[^>]*value=\"([^\"]+)\"")]
    private static partial Regex TokenRegex();

    [GeneratedRegex("<form[^>]*action=\"([^\"]*)\"", RegexOptions.IgnoreCase)]
    private static partial Regex FormActionRegex();
}
