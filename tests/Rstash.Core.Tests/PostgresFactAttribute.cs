using System.Net.Sockets;
using Npgsql;

namespace Rstash.Core.Tests;

/// <summary>
/// A <see cref="FactAttribute"/> that self-skips when a PostgreSQL server isn't
/// reachable — the DB analog of <see cref="AzuriteFactAttribute"/>. Lets the Postgres
/// provider round-trip tests run locally/CI when a server is up, and report an honest
/// "Skipped" (not a false pass) when it isn't.
/// </summary>
public sealed class PostgresFactAttribute : FactAttribute
{
    public PostgresFactAttribute()
    {
        if (!PostgresServer.IsReachable)
        {
            Skip = $"PostgreSQL not reachable at {PostgresServer.Host}:{PostgresServer.Port} " +
                   "(set RSTASH_TEST_POSTGRES to a native Npgsql connection string).";
        }
    }
}

/// <summary>
/// Connection details for the test PostgreSQL server and scratch-database lifecycle.
/// Defaults to a local server with the conventional <c>postgres/postgres</c> dev
/// credentials; override the whole connection string via the
/// <c>RSTASH_TEST_POSTGRES</c> environment variable.
/// </summary>
public static class PostgresServer
{
    /// <summary>A native Npgsql connection string to the server (any database).</summary>
    public static string BaseConnectionString =>
        Environment.GetEnvironmentVariable("RSTASH_TEST_POSTGRES")
        ?? "Host=127.0.0.1;Port=5432;Username=postgres;Password=postgres";

    public static string Host => new NpgsqlConnectionStringBuilder(BaseConnectionString).Host ?? "127.0.0.1";

    public static int Port => new NpgsqlConnectionStringBuilder(BaseConnectionString).Port;

    private static readonly Lazy<bool> Reachable = new(Probe);

    public static bool IsReachable => Reachable.Value;

    /// <summary>
    /// Creates a uniquely-named scratch database and returns an rstash <c>postgres:</c>
    /// DSN pointing at it. Each test gets its own database for clean isolation.
    /// </summary>
    public static string CreateScratchDatabase()
    {
        var name = "rstash_test_" + Guid.NewGuid().ToString("N");

        using (var connection = new NpgsqlConnection(MaintenanceConnectionString()))
        {
            connection.Open();
            using var command = connection.CreateCommand();
            command.CommandText = $"CREATE DATABASE \"{name}\"";
            command.ExecuteNonQuery();
        }

        var builder = new NpgsqlConnectionStringBuilder(BaseConnectionString) { Database = name };
        return "postgres:" + builder.ConnectionString;
    }

    /// <summary>Drops a scratch database created by <see cref="CreateScratchDatabase"/>.</summary>
    public static void DropScratchDatabase(string dsn)
    {
        var connectionString = dsn["postgres:".Length..];
        var name = new NpgsqlConnectionStringBuilder(connectionString).Database!;

        NpgsqlConnection.ClearAllPools();
        using var connection = new NpgsqlConnection(MaintenanceConnectionString());
        connection.Open();
        using var command = connection.CreateCommand();
        // WITH (FORCE) terminates lingering connections (Postgres 13+).
        command.CommandText = $"DROP DATABASE IF EXISTS \"{name}\" WITH (FORCE)";
        command.ExecuteNonQuery();
    }

    private static string MaintenanceConnectionString() =>
        new NpgsqlConnectionStringBuilder(BaseConnectionString) { Database = "postgres" }.ConnectionString;

    private static bool Probe()
    {
        try
        {
            using var client = new TcpClient();
            return client.ConnectAsync(Host, Port).Wait(TimeSpan.FromMilliseconds(500))
                && client.Connected;
        }
        catch
        {
            return false;
        }
    }
}
