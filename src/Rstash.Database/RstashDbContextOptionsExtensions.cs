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
            Dialect.Sqlite => builder.UseSqlite(ToSqliteConnectionString(parsed.ConnectionString)),
            _ => throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not yet wired in the .NET rewrite; " +
                "only sqlite is implemented so far."),
        };
    }

    private static string ToSqliteConnectionString(string pathOrConnection) =>
        pathOrConnection.Contains('=', StringComparison.Ordinal)
            ? pathOrConnection // already a Microsoft.Data.Sqlite connection string
            : $"Data Source={pathOrConnection}";
}
