namespace Rstash.Database;

/// <summary>
/// A parsed database DSN: the dialect plus the provider connection string.
/// Supported schemes are <c>sqlite:</c>, <c>postgres:</c>, <c>mysql:</c>,
/// <c>mssql:</c>; a bare value with no scheme is treated as a SQLite path.
/// </summary>
public readonly record struct DatabaseDsn(Dialect Dialect, string ConnectionString)
{
    private static readonly (string Prefix, Dialect Dialect)[] Schemes =
    [
        ("sqlite:", Dialect.Sqlite),
        ("postgres:", Dialect.Postgres),
        ("mysql:", Dialect.MySql),
        ("mssql:", Dialect.SqlServer),
    ];

    public static DatabaseDsn Parse(string dsn)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(dsn);

        foreach (var (prefix, dialect) in Schemes)
        {
            if (!dsn.StartsWith(prefix, StringComparison.Ordinal))
            {
                continue;
            }

            var remainder = dsn[prefix.Length..];

            // The Postgres/MySQL drivers accept a full URL (e.g.
            // postgres://user:pass@host/db). When the remainder already looks
            // like one, hand back the original DSN so the scheme stays intact.
            if (dialect is Dialect.Postgres or Dialect.MySql
                && remainder.StartsWith("//", StringComparison.Ordinal))
            {
                return new DatabaseDsn(dialect, dsn);
            }

            return new DatabaseDsn(dialect, remainder);
        }

        // No recognised scheme — treat the whole value as a SQLite path.
        return new DatabaseDsn(Dialect.Sqlite, dsn);
    }
}
