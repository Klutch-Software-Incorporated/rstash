using System.Data.Common;
using System.Text;
using Microsoft.EntityFrameworkCore.Diagnostics;

namespace Rstash.Database;

/// <summary>
/// Applies rstash's standard PRAGMAs to every opened SQLite connection.
/// <para>
/// <c>journal_mode</c> is taken from the DSN (see
/// <see cref="RstashDbContextOptionsExtensions.SqliteJournalMode"/>) rather than
/// hard-coded, because it is environment-specific: the hosted deployment stores
/// its SQLite files on an Azure Files (SMB) share, where WAL mode corrupts the
/// database (its <c>-shm</c> file can't be memory-mapped over SMB) and
/// <c>journal_mode=delete</c> is required. When the DSN specifies no mode we leave
/// SQLite's safe <c>delete</c> default untouched — we never force WAL.
/// </para>
/// <para>
/// The remaining pragmas are safe everywhere and always applied:
/// <c>busy_timeout</c> (wait rather than fail on a briefly locked DB),
/// <c>foreign_keys=ON</c>, and <c>case_sensitive_like=ON</c> (path-prefix LIKE
/// queries stay case-sensitive).
/// </para>
/// </summary>
internal sealed class SqliteConnectionInitInterceptor : DbConnectionInterceptor
{
    // SQLite's supported journal modes (https://www.sqlite.org/pragma.html#pragma_journal_mode).
    private static readonly HashSet<string> ValidJournalModes =
        new(StringComparer.OrdinalIgnoreCase) { "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "WAL", "OFF" };

    private readonly string _pragmas;

    /// <param name="journalMode">
    /// The <c>journal_mode</c> from the DSN, or <c>null</c> to leave SQLite's
    /// default in place. Validated against <see cref="ValidJournalModes"/> so the
    /// value is safe to inline into the PRAGMA statement.
    /// </param>
    public SqliteConnectionInitInterceptor(string? journalMode)
    {
        var builder = new StringBuilder();

        if (journalMode is not null)
        {
            if (!ValidJournalModes.Contains(journalMode))
            {
                throw new ArgumentException(
                    $"Unsupported SQLite journal_mode '{journalMode}'. " +
                    $"Valid values: {string.Join(", ", ValidJournalModes)}.",
                    nameof(journalMode));
            }

            builder.Append($"PRAGMA journal_mode={journalMode};");
        }

        builder.Append("PRAGMA busy_timeout=5000;PRAGMA foreign_keys=ON;PRAGMA case_sensitive_like=ON;");
        _pragmas = builder.ToString();
    }

    public override void ConnectionOpened(DbConnection connection, ConnectionEndEventData eventData) =>
        Apply(connection);

    public override Task ConnectionOpenedAsync(
        DbConnection connection, ConnectionEndEventData eventData, CancellationToken cancellationToken = default)
    {
        Apply(connection);
        return Task.CompletedTask;
    }

    private void Apply(DbConnection connection)
    {
        using var command = connection.CreateCommand();
        command.CommandText = _pragmas;
        command.ExecuteNonQuery();
    }
}
