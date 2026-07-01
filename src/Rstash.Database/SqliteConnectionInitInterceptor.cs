using System.Data.Common;
using Microsoft.EntityFrameworkCore.Diagnostics;

namespace Rstash.Database;

/// <summary>
/// Applies rstash's standard PRAGMAs to every opened SQLite connection, matching
/// the Go server's initialization (<c>legacy/internal/db/gormdb.go</c>):
/// <list type="bullet">
///   <item><c>journal_mode=WAL</c> — better read/write concurrency for a server
///   workload (persisted in the database file; harmless to re-assert on open).</item>
///   <item><c>busy_timeout=5000</c> — wait rather than fail on a briefly locked DB.</item>
///   <item><c>foreign_keys=ON</c> — enforce FK constraints where they exist.</item>
///   <item><c>case_sensitive_like=ON</c> — path-prefix (LIKE) queries stay
///   case-sensitive.</item>
/// </list>
/// This is where <c>journal_mode=WAL</c> belongs — it is a pragma, not a
/// connection-string keyword, so it can't be carried in the DSN (see
/// <see cref="RstashDbContextOptionsExtensions.ToSqliteConnectionString"/>).
/// </summary>
internal sealed class SqliteConnectionInitInterceptor : DbConnectionInterceptor
{
    public static readonly SqliteConnectionInitInterceptor Instance = new();

    private const string Pragmas =
        "PRAGMA journal_mode=WAL;" +
        "PRAGMA busy_timeout=5000;" +
        "PRAGMA foreign_keys=ON;" +
        "PRAGMA case_sensitive_like=ON;";

    public override void ConnectionOpened(DbConnection connection, ConnectionEndEventData eventData) =>
        Apply(connection);

    public override Task ConnectionOpenedAsync(
        DbConnection connection, ConnectionEndEventData eventData, CancellationToken cancellationToken = default)
    {
        Apply(connection);
        return Task.CompletedTask;
    }

    private static void Apply(DbConnection connection)
    {
        using var command = connection.CreateCommand();
        command.CommandText = Pragmas;
        command.ExecuteNonQuery();
    }
}
