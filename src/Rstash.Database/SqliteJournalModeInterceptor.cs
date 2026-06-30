using System.Data.Common;
using Microsoft.EntityFrameworkCore.Diagnostics;

namespace Rstash.Database;

/// <summary>
/// Sets <c>PRAGMA journal_mode</c> on every opened SQLite connection. This is an
/// operator opt-in via the <c>?journal_mode=</c> DSN parameter — it is NOT applied
/// by default, so a normal local-disk deployment keeps SQLite's own default.
/// <para>
/// The reason it exists: SQLite's WAL mode needs a memory-mapped <c>-shm</c> file
/// that does not work on network/SMB file shares (e.g. Azure Files, which backs
/// <c>/home</c> on Azure App Service). There it leaves the database half-written
/// and migrations fail on boot. Such deployments set <c>journal_mode=delete</c>
/// (a rollback journal, which uses ordinary byte-range locks) to work around it.
/// </para>
/// </summary>
internal sealed class SqliteJournalModeInterceptor(string journalMode) : DbConnectionInterceptor
{
    // Caller validates against the allow-list, so interpolating here is safe
    // (PRAGMA values cannot be parameterized).
    private readonly string _pragma = $"PRAGMA journal_mode = {journalMode};";

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
        command.CommandText = _pragma;
        command.ExecuteNonQuery();
    }
}
