using FluentMigrator;
using FluentMigrator.Runner;
using FluentMigrator.Runner.Generators.Generic;
using FluentMigrator.Runner.Generators.SQLite;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;
using Rstash.Database.Migrations;

namespace Rstash.Database;

/// <summary>
/// Applies the FluentMigrator schema (see <see cref="InitialCreate"/>) to the
/// configured database, bringing it up to the latest version.
/// </summary>
/// <remarks>
/// This owns DDL in place of EF Core's <c>Database.Migrate()</c>: the host runs it once
/// at startup, and the tests reuse it to build a schema before exercising the EF stores.
/// SQLite and Postgres are wired; MySQL and SQL Server are not yet supported.
/// </remarks>
public static class SchemaMigrator
{
    public static void MigrateUp(string dsn) => MigrateUp(dsn, targetVersion: null);

    /// <param name="targetVersion">
    /// Stop after this migration instead of applying everything. Only tests use this,
    /// to build a database at an older schema and then exercise a data-migrating step
    /// against rows that actually exist.
    /// </param>
    internal static void MigrateUp(string dsn, long? targetVersion)
    {
        var parsed = DatabaseDsn.Parse(dsn);

        var connectionString = parsed.Dialect switch
        {
            Dialect.Sqlite => RstashDbContextOptionsExtensions.ToSqliteConnectionString(parsed.ConnectionString),
            Dialect.Postgres => PostgresConnectionString(parsed.ConnectionString),
            _ => throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not supported; " +
                "only sqlite and postgres are wired."),
        };

        var isSqlite = parsed.Dialect is Dialect.Sqlite;

        var services = new ServiceCollection()
            .AddFluentMigratorCore()
            .ConfigureRunner(rb => (isSqlite ? rb.AddSQLite() : rb.AddPostgres())
                .WithGlobalConnectionString(connectionString)
                .ScanIn(typeof(InitialCreate).Assembly).For.Migrations())
            .AddLogging(lb => lb.AddFluentMigratorConsole());

        // SQLite-only: teach the generator that DateTimeOffset maps to TEXT (see
        // RstashSQLiteTypeMap). This override must follow AddSQLite() to win, and it
        // must not be registered for other dialects (it is an ISQLiteTypeMap).
        if (isSqlite)
        {
            services.AddScoped<ISQLiteTypeMap, RstashSQLiteTypeMap>();
        }

        using var serviceProvider = services.BuildServiceProvider(validateScopes: false);
        using var scope = serviceProvider.CreateScope();

        // SQLite-only: it cannot ALTER TABLE ADD a standalone foreign key (and enforces
        // no FKs by default anyway). LOOSE mode makes the SQLite generator skip those
        // unsupported statements. Postgres keeps the default STRICT generator so the
        // same agnostic migration creates real, enforced FKs.
        if (isSqlite
            && scope.ServiceProvider.GetRequiredService<IMigrationGenerator>() is GenericGenerator generator)
        {
            generator.CompatibilityMode = CompatibilityMode.LOOSE;
        }

        var runner = scope.ServiceProvider.GetRequiredService<IMigrationRunner>();
        if (targetVersion is { } version)
        {
            runner.MigrateUp(version);
        }
        else
        {
            runner.MigrateUp();
        }
    }

    /// <summary>
    /// Resolves the Npgsql connection string for the migration run. Unlike the EF
    /// path, FluentMigrator's runner takes a plain connection string (no
    /// <c>NpgsqlDataSource</c> seam), so for <c>Auth=Entra</c> we inject a single
    /// access token as the password — one token easily covers the sub-second run.
    /// </summary>
    private static string PostgresConnectionString(string connection)
    {
        var (connectionString, useEntra) = PostgresDsn.Parse(connection);
        if (!useEntra)
        {
            return connectionString;
        }

        return new NpgsqlConnectionStringBuilder(connectionString)
        {
            Password = EntraPostgres.FetchToken(),
        }.ConnectionString;
    }
}
