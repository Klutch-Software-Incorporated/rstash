using System.Data.Common;
using Microsoft.EntityFrameworkCore.Diagnostics;

namespace Rstash.Database;

/// <summary>
/// Sets <c>PRAGMA case_sensitive_like = ON</c> on every opened SQLite
/// connection so path-prefix (LIKE) queries are case-sensitive, matching the
/// Go server's behavior.
/// </summary>
internal sealed class SqliteCaseSensitiveLikeInterceptor : DbConnectionInterceptor
{
    public static readonly SqliteCaseSensitiveLikeInterceptor Instance = new();

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
        command.CommandText = "PRAGMA case_sensitive_like = ON;";
        command.ExecuteNonQuery();
    }
}
