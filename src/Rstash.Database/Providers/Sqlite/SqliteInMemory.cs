using System.Collections.Concurrent;
using Microsoft.Data.Sqlite;

namespace Rstash.Database;

/// <summary>
/// Support for shareable in-memory SQLite databases — handy for local development and
/// tests that want a clean database on every process start.
/// <para>
/// A plain <c>:memory:</c> database is private to a single connection and is destroyed
/// the moment that connection closes. rstash opens SQLite through several short-lived
/// connections (the FluentMigrator schema run, then EF per operation), so a naive
/// <c>:memory:</c> would lose its schema before the first query. This helper instead maps
/// an in-memory DSN to a <em>named, shared-cache</em> database and holds one process-
/// lifetime "keep-alive" connection open, so every later connection sees the same live
/// database. Restarting the process starts from an empty database — which is the point.
/// </para>
/// </summary>
internal static class SqliteInMemory
{
    // One keep-alive connection per in-memory database, held for the process lifetime.
    // A shared-cache in-memory database vanishes when its LAST connection closes; keeping
    // one open here is what lets the schema built by the migrator survive to be queried.
    private static readonly ConcurrentDictionary<string, SqliteConnection> KeepAlives = new();

    // The database name used for a bare "sqlite::memory:" DSN (no explicit name).
    private const string DefaultName = "rstash";

    private const string MemoryPrefix = ":memory:";

    /// <summary>
    /// True when a <c>sqlite:</c> DSN remainder names an in-memory database — either the
    /// <c>:memory:</c> sentinel (optionally followed by a name, e.g. <c>:memory:blobs</c>)
    /// or an explicit <c>Mode=Memory</c> connection string.
    /// </summary>
    internal static bool IsInMemory(string pathOrConnection) =>
        pathOrConnection.StartsWith(MemoryPrefix, StringComparison.Ordinal)
        || pathOrConnection.Contains("Mode=Memory", StringComparison.OrdinalIgnoreCase);

    /// <summary>
    /// Builds the shared-cache connection string for an in-memory DSN remainder — a pure
    /// mapping with no side effects, so callers can compare two DSNs for equality (see
    /// <see cref="RstashDbContextOptionsExtensions.EnsureDistinctInMemoryDatabases"/>).
    /// The database name is the text after <c>:memory:</c> (so distinct DSNs get distinct
    /// databases); a bare <c>:memory:</c> uses <see cref="DefaultName"/>. An already-formed
    /// <c>Mode=Memory</c> connection string is returned unchanged.
    /// </summary>
    internal static string SharedConnectionString(string pathOrConnection)
    {
        if (!pathOrConnection.StartsWith(MemoryPrefix, StringComparison.Ordinal))
        {
            return pathOrConnection; // already an explicit Mode=Memory connection string
        }

        var name = pathOrConnection[MemoryPrefix.Length..];
        var queryStart = name.IndexOf('?', StringComparison.Ordinal);
        if (queryStart >= 0)
        {
            name = name[..queryStart];
        }

        return new SqliteConnectionStringBuilder
        {
            DataSource = string.IsNullOrEmpty(name) ? DefaultName : name,
            Mode = SqliteOpenMode.Memory,
            Cache = SqliteCacheMode.Shared,
        }.ConnectionString;
    }

    /// <summary>
    /// Resolves the shared-cache connection string for an in-memory DSN remainder and
    /// ensures a keep-alive connection is open for it, so the database outlives the
    /// individual connections that migrate and query it. Idempotent per database.
    /// </summary>
    internal static string Resolve(string pathOrConnection)
    {
        var connectionString = SharedConnectionString(pathOrConnection);
        KeepAlives.GetOrAdd(connectionString, OpenKeepAlive);
        return connectionString;
    }

    private static SqliteConnection OpenKeepAlive(string connectionString)
    {
        var connection = new SqliteConnection(connectionString);
        connection.Open();
        return connection;
    }
}
