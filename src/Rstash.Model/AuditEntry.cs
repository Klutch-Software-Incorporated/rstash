namespace Rstash.Model;

/// <summary>
/// An append-only record of a state-changing operation (admin, storage, auth,
/// OAuth). Persisted to the <c>audit_log</c> table (mapping configured in the
/// database layer).
/// </summary>
public class AuditEntry
{
    public long Id { get; set; }

    /// <summary>User id that performed the action.</summary>
    public long ActorId { get; set; }

    public string Action { get; set; } = string.Empty;

    public string TargetType { get; set; } = string.Empty;

    public string TargetId { get; set; } = string.Empty;

    public string Details { get; set; } = string.Empty;

    public DateTimeOffset CreatedAt { get; set; }
}
