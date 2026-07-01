using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace Rstash.Database;

/// <summary>
/// Configures a <see cref="RstashDbContext"/> from an rstash DSN. Only SQLite is
/// wired today; the other dialects (Postgres/MySQL/SQL Server) are added with
/// their provider packages in a later step.
/// </summary>
public static class RstashDbContextOptionsExtensions
{
    /// <summary>Typed overload that preserves the generic builder (used by the
    /// design-time factory and <c>AddDbContext&lt;RstashDbContext&gt;</c>).</summary>
    public static DbContextOptionsBuilder<TContext> UseRstashDatabase<TContext>(
        this DbContextOptionsBuilder<TContext> builder, string dsn)
        where TContext : DbContext
    {
        UseRstashDatabase((DbContextOptionsBuilder)builder, dsn);
        return builder;
    }

    public static DbContextOptionsBuilder UseRstashDatabase(
        this DbContextOptionsBuilder builder, string dsn)
    {
        var parsed = DatabaseDsn.Parse(dsn);

        return parsed.Dialect switch
        {
            Dialect.Sqlite => builder
                .UseSqlite(ToSqliteConnectionString(parsed.ConnectionString))
                .AddInterceptors(new SqliteConnectionInitInterceptor(SqliteJournalMode(parsed.ConnectionString))),
            _ => throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not yet wired in the .NET rewrite; " +
                "only sqlite is implemented so far."),
        };
    }

    /// <summary>
    /// Converts a SQLite DSN remainder to a Microsoft.Data.Sqlite connection
    /// string. Shared with <see cref="SchemaMigrator"/> so EF and FluentMigrator
    /// open the same database the same way.
    /// </summary>
    /// <remarks>
    /// The remainder is normally a filesystem path (or <c>:memory:</c>), but the
    /// DSN may append URI-style query parameters such as
    /// <c>?journal_mode=delete</c>. Those are pragmas, not connection-string
    /// keywords — Microsoft.Data.Sqlite would reject them — so the query is
    /// stripped here and any pragmas are applied on open by
    /// <see cref="SqliteConnectionInitInterceptor"/> (see
    /// <see cref="SqliteJournalMode"/>). A value that already looks like a
    /// Microsoft.Data.Sqlite connection string (contains <c>Data Source</c>) is
    /// passed through unchanged.
    /// </remarks>
    internal static string ToSqliteConnectionString(string pathOrConnection)
    {
        if (pathOrConnection.Contains("Data Source", StringComparison.OrdinalIgnoreCase))
        {
            return pathOrConnection;
        }

        var path = pathOrConnection;
        var queryStart = path.IndexOf('?', StringComparison.Ordinal);
        if (queryStart >= 0)
        {
            path = path[..queryStart];
        }

        return new SqliteConnectionStringBuilder { DataSource = path }.ConnectionString;
    }

    /// <summary>
    /// Reads the <c>journal_mode</c> query parameter from a SQLite DSN remainder,
    /// or <c>null</c> when absent. This is environment-critical: the hosted
    /// deployment stores its SQLite files on an Azure Files (SMB) share and must
    /// run <c>journal_mode=delete</c> — WAL corrupts the database over SMB. The
    /// value is honored (and validated) by <see cref="SqliteConnectionInitInterceptor"/>.
    /// </summary>
    internal static string? SqliteJournalMode(string pathOrConnection)
    {
        var queryStart = pathOrConnection.IndexOf('?', StringComparison.Ordinal);
        if (queryStart < 0)
        {
            return null;
        }

        foreach (var pair in pathOrConnection[(queryStart + 1)..].Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var eq = pair.IndexOf('=', StringComparison.Ordinal);
            var key = eq < 0 ? pair : pair[..eq];
            if (key.Equals("journal_mode", StringComparison.OrdinalIgnoreCase))
            {
                return eq < 0 ? "" : pair[(eq + 1)..];
            }
        }

        return null;
    }
}
