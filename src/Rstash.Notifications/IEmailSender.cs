namespace Rstash.Notifications;

/// <summary>An outbound email message.</summary>
public sealed record EmailMessage
{
    public required string To { get; init; }

    public required string Subject { get; init; }

    public string? Html { get; init; }

    public string? Text { get; init; }

    /// <summary>Optional override for the configured "from" address.</summary>
    public string? From { get; init; }
}

/// <summary>Sends transactional email. The contract lives with its implementations.</summary>
public interface IEmailSender
{
    /// <summary>False when no provider is configured (sends are no-ops).</summary>
    bool IsConfigured { get; }

    Task SendAsync(EmailMessage message, CancellationToken cancellationToken = default);
}
