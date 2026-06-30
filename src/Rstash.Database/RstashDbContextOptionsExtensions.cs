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
            Dialect.Sqlite => UseSqlite(builder, parsed.ConnectionString),
            _ => throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not yet wired in the .NET rewrite; " +
                "only sqlite is implemented so far."),
        };
    }

    private static DbContextOptionsBuilder UseSqlite(DbContextOptionsBuilder builder, string spec)
    {
        var (pathOrConnection, journalMode) = SplitSqliteSpec(spec);

        builder
            .UseSqlite(ToSqliteConnectionString(pathOrConnection))
            .AddInterceptors(SqliteCaseSensitiveLikeInterceptor.Instance);

        // Operator opt-in, applied only when ?journal_mode= is on the DSN: lets a
        // network/SMB-share deployment (e.g. Azure Files) request journal_mode=delete,
        // where SQLite's WAL mode doesn't work. Local-disk users keep the default.
        if (journalMode is not null)
        {
            builder.AddInterceptors(new SqliteJournalModeInterceptor(journalMode));
        }

        return builder;
    }

    private static readonly HashSet<string> ValidJournalModes =
        new(StringComparer.Ordinal) { "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "WAL", "OFF" };

    /// <summary>Splits an optional <c>?journal_mode=&lt;mode&gt;</c> off a SQLite path.</summary>
    private static (string PathOrConnection, string? JournalMode) SplitSqliteSpec(string spec)
    {
        var query = spec.IndexOf('?', StringComparison.Ordinal);
        if (query < 0)
        {
            return (spec, null);
        }

        var pathOrConnection = spec[..query];
        string? journalMode = null;
        foreach (var part in spec[(query + 1)..].Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var eq = part.IndexOf('=', StringComparison.Ordinal);
            if (eq <= 0 || !part[..eq].Trim().Equals("journal_mode", StringComparison.OrdinalIgnoreCase))
            {
                continue;
            }

            journalMode = part[(eq + 1)..].Trim().ToUpperInvariant();
            if (!ValidJournalModes.Contains(journalMode))
            {
                throw new ArgumentException(
                    $"invalid SQLite journal_mode '{journalMode}' " +
                    $"(expected one of: {string.Join(", ", ValidJournalModes)})");
            }
        }

        return (pathOrConnection, journalMode);
    }

    private static string ToSqliteConnectionString(string pathOrConnection) =>
        pathOrConnection.Contains('=', StringComparison.Ordinal)
            ? pathOrConnection // already a Microsoft.Data.Sqlite connection string
            : $"Data Source={pathOrConnection}";
}
