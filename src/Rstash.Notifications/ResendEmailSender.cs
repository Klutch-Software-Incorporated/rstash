using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Rstash.Notifications;

/// <summary>Sends email via the Resend HTTP API.</summary>
public sealed class ResendEmailSender(string apiKey, string from) : IEmailSender
{
    private static readonly HttpClient Http = new();

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    public bool IsConfigured => true;

    public async Task SendAsync(EmailMessage message, CancellationToken cancellationToken = default)
    {
        var payload = new
        {
            from = message.From ?? from,
            to = new[] { message.To },
            subject = message.Subject,
            html = message.Html,
            text = message.Text,
        };

        using var request = new HttpRequestMessage(HttpMethod.Post, "https://api.resend.com/emails")
        {
            Content = JsonContent.Create(payload, options: JsonOptions),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);

        using var response = await Http.SendAsync(request, cancellationToken);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken);
            throw new InvalidOperationException($"Resend API error ({(int)response.StatusCode}): {body}");
        }
    }
}
