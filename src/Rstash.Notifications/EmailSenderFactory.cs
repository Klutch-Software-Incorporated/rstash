namespace Rstash.Notifications;

/// <summary>
/// Builds an <see cref="IEmailSender"/> from a DSN. Format:
/// <c>resend:API_KEY?from=noreply@example.com</c>. An empty/invalid DSN yields
/// a no-op sender (email features stay disabled).
/// </summary>
public static class EmailSenderFactory
{
    public static IEmailSender Create(string? dsn)
    {
        if (string.IsNullOrEmpty(dsn))
        {
            return new NoOpEmailSender();
        }

        var colon = dsn.IndexOf(':', StringComparison.Ordinal);
        if (colon < 1)
        {
            return new NoOpEmailSender();
        }

        var scheme = dsn[..colon];
        var rest = dsn[(colon + 1)..];
        if (scheme != "resend")
        {
            return new NoOpEmailSender();
        }

        var apiKey = rest;
        string? from = null;

        var query = rest.IndexOf('?', StringComparison.Ordinal);
        if (query >= 0)
        {
            apiKey = rest[..query];
            foreach (var pair in rest[(query + 1)..].Split('&'))
            {
                var kv = pair.Split('=', 2);
                if (kv.Length == 2 && kv[0] == "from")
                {
                    from = Uri.UnescapeDataString(kv[1]);
                }
            }
        }

        return string.IsNullOrEmpty(apiKey) || string.IsNullOrEmpty(from)
            ? new NoOpEmailSender()
            : new ResendEmailSender(apiKey, from);
    }
}
