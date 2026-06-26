namespace Rstash.Notifications;

/// <summary>Used when no email provider is configured; sends are discarded.</summary>
public sealed class NoOpEmailSender : IEmailSender
{
    public bool IsConfigured => false;

    public Task SendAsync(EmailMessage message, CancellationToken cancellationToken = default) =>
        Task.CompletedTask;
}
