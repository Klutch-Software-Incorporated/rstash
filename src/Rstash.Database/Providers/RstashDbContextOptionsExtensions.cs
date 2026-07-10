using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace Rstash.Database;

/// <summary>
/// Configures a <see cref="RstashDbContext"/> from an rstash DSN. SQLite and Postgres
/// (including Azure Entra ID auth) are wired; MySQL and SQL Server are not yet
/// supported.
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
            Dialect.Postgres => ConfigurePostgres(builder, parsed.ConnectionString),
            // MySQL and SQL Server aren't wired yet; they fall through to the throw below.
            _ => throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not supported; " +
                "only sqlite and postgres are wired."),
        };
    }

    /// <summary>
    /// Configures the Npgsql provider from the connection portion of a <c>postgres:</c>
    /// DSN. Postgres <c>LIKE</c> is case-sensitive by default, so (unlike SQLite) no
    /// connection interceptor is needed to keep path-prefix queries case-sensitive.
    /// </summary>
    private static DbContextOptionsBuilder ConfigurePostgres(DbContextOptionsBuilder builder, string connection)
    {
        var (connectionString, useEntra) = PostgresDsn.Parse(connection);

        return useEntra
            ? builder.UseNpgsql(EntraPostgres.BuildDataSource(connectionString))
            : builder.UseNpgsql(connectionString);
    }

    /// <summary>
    /// Converts a SQLite DSN remainder to a Microsoft.Data.Sqlite connection
    /// string. Shared with <see cref="SchemaMigrator"/> so EF and FluentMigrator
    /// open the same database the same way.
    /// </summary>
    /// <remarks>
    /// The remainder is normally a filesystem path, but it may also be an in-memory
    /// database (<c>:memory:</c>, optionally named as <c>:memory:blobs</c>), which
    /// <see cref="SqliteInMemory"/> maps to a shared-cache database with a
    /// process-lifetime keep-alive connection so it survives across opens. The DSN may
    /// also append URI-style query parameters such as <c>?journal_mode=delete</c>.
    /// Those are pragmas, not connection-string keywords — Microsoft.Data.Sqlite would
    /// reject them — so the query is stripped here and any pragmas are applied on open
    /// by <see cref="SqliteConnectionInitInterceptor"/> (see
    /// <see cref="SqliteJournalMode"/>). A value that already looks like a
    /// Microsoft.Data.Sqlite connection string (contains <c>Data Source</c>) is
    /// passed through unchanged.
    /// </remarks>
    internal static string ToSqliteConnectionString(string pathOrConnection)
    {
        // In-memory databases need a shared cache plus a keep-alive connection, or the
        // schema would vanish between the migrator and the first EF query.
        if (SqliteInMemory.IsInMemory(pathOrConnection))
        {
            return SqliteInMemory.Resolve(pathOrConnection);
        }

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

    /// <summary>
    /// Throws when the metadata and blob DSNs point at the SAME in-memory SQLite
    /// database. An in-memory database is per-process and holds a single schema, so
    /// sharing one between the metadata tables and the blob table makes blob
    /// initialization fail confusingly. File-backed and server DSNs are always allowed
    /// (they can safely share a server, and even a file, without clashing).
    /// </summary>
    public static void EnsureDistinctInMemoryDatabases(string metadataDsn, string blobDsn)
    {
        var meta = DatabaseDsn.Parse(metadataDsn);
        var blob = DatabaseDsn.Parse(blobDsn);

        if (meta.Dialect is not Dialect.Sqlite || blob.Dialect is not Dialect.Sqlite
            || !SqliteInMemory.IsInMemory(meta.ConnectionString)
            || !SqliteInMemory.IsInMemory(blob.ConnectionString))
        {
            return;
        }

        if (string.Equals(
                SqliteInMemory.SharedConnectionString(meta.ConnectionString),
                SqliteInMemory.SharedConnectionString(blob.ConnectionString),
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                "RSTASH_DB and RSTASH_BLOB point at the same in-memory SQLite database, which " +
                "cannot hold both schemas. Give them distinct names, e.g. RSTASH_DB=sqlite::memory: " +
                "and RSTASH_BLOB=sqlite::memory:blobs.");
        }
    }
}
