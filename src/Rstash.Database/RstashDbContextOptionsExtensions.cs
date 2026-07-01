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
                .AddInterceptors(SqliteConnectionInitInterceptor.Instance),
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
    /// The remainder is normally a filesystem path (or <c>:memory:</c>), but DSNs
    /// carried over from the Go server may append URI-style query parameters such
    /// as <c>?journal_mode=WAL</c>. Those are pragmas, not connection-string
    /// keywords — Microsoft.Data.Sqlite would reject them — so the query is
    /// stripped here and the pragmas are applied on open by
    /// <see cref="SqliteConnectionInitInterceptor"/>. A value that already looks
    /// like a Microsoft.Data.Sqlite connection string (contains <c>Data Source</c>)
    /// is passed through unchanged.
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
}
