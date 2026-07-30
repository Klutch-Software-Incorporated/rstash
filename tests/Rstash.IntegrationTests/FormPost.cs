using System.Text.RegularExpressions;

namespace Rstash.IntegrationTests;

/// <summary>
/// Completes an OpenID Connect <c>form_post</c> response.
/// </summary>
/// <remarks>
/// The handler asks for <c>response_mode=form_post</c>, so the provider answers the
/// authorize request with a 200 HTML page containing a self-submitting form rather
/// than a redirect. A browser runs it; <see cref="HttpClient"/> does not, and simply
/// stops there with a perfectly innocent-looking 200. Any assertion that only checks
/// the status code — or that the URL is not the login page — passes without the login
/// ever having completed.
/// </remarks>
internal static partial class FormPost
{
    /// <summary>True when a response is a form_post page rather than real content.</summary>
    public static bool IsFormPost(string html) =>
        html.Contains("<form", StringComparison.OrdinalIgnoreCase)
        && html.Contains("name=\"state\"", StringComparison.OrdinalIgnoreCase);

    /// <summary>Submits the self-posting form and returns the resulting response.</summary>
    public static async Task<HttpResponseMessage> SubmitAsync(HttpClient client, string html)
    {
        var action = ActionRegex().Match(html).Groups[1].Value;
        var fields = InputRegex()
            .Matches(html)
            .ToDictionary(m => m.Groups[1].Value, m => System.Net.WebUtility.HtmlDecode(m.Groups[2].Value));

        return await client.PostAsync(new Uri(action).PathAndQuery, new FormUrlEncodedContent(fields));
    }

    [GeneratedRegex("<form[^>]*action=[\"']([^\"']+)[\"']", RegexOptions.IgnoreCase)]
    private static partial Regex ActionRegex();

    [GeneratedRegex("<input[^>]*name=[\"']([^\"']+)[\"'][^>]*value=[\"']([^\"']*)[\"']", RegexOptions.IgnoreCase)]
    private static partial Regex InputRegex();
}
