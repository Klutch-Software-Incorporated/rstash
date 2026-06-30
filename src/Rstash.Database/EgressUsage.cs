namespace Rstash.Database;

/// <summary>
/// Accumulated outbound transfer (egress) for one user within one monthly
/// billing period ("YYYY-MM", UTC). One row per (user, period); the counter is
/// incremented as documents are served and reset implicitly when the period rolls.
/// </summary>
public sealed class EgressUsage
{
    public long Id { get; set; }

    public long UserId { get; set; }

    /// <summary>UTC period this row counts, formatted "YYYY-MM".</summary>
    public string Period { get; set; } = "";

    /// <summary>Bytes served to the user during the period.</summary>
    public long BytesOut { get; set; }

    public DateTimeOffset UpdatedAt { get; set; }
}
