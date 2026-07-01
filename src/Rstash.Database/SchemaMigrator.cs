using FluentMigrator;
using FluentMigrator.Runner;
using FluentMigrator.Runner.Generators.Generic;
using FluentMigrator.Runner.Generators.SQLite;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database.Migrations;

namespace Rstash.Database;

/// <summary>
/// Applies the FluentMigrator schema (see <see cref="InitialCreate"/>) to the
/// configured database, bringing it up to the latest version. This replaces EF
/// Core's <c>Database.Migrate()</c> as the owner of DDL; the host runs it once at
/// startup and the tests reuse it to build a schema before exercising the EF
/// stores.
///
/// Only SQLite is wired today (the sole runtime provider). The Postgres/SQL
/// Server/MySQL generators are registered here alongside their EF providers in a
/// later branch — a matching <c>Dialect</c> arm plus the generator's
/// <c>.Add…()</c> call.
/// </summary>
public static class SchemaMigrator
{
    public static void MigrateUp(string dsn)
    {
        var parsed = DatabaseDsn.Parse(dsn);
        if (parsed.Dialect is not Dialect.Sqlite)
        {
            throw new NotSupportedException(
                $"Database dialect '{parsed.Dialect}' is not yet wired for migrations; " +
                "only sqlite is implemented so far.");
        }

        var connectionString = RstashDbContextOptionsExtensions.ToSqliteConnectionString(parsed.ConnectionString);

        var services = new ServiceCollection()
            .AddFluentMigratorCore()
            .ConfigureRunner(rb => rb
                .AddSQLite()
                .WithGlobalConnectionString(connectionString)
                .ScanIn(typeof(InitialCreate).Assembly).For.Migrations())
            .AddLogging(lb => lb.AddFluentMigratorConsole());

        // Teach the SQLite generator that DateTimeOffset maps to TEXT (see
        // RstashSQLiteTypeMap); this override must follow AddSQLite() to win.
        services.AddScoped<ISQLiteTypeMap, RstashSQLiteTypeMap>();

        using var serviceProvider = services.BuildServiceProvider(validateScopes: false);
        using var scope = serviceProvider.CreateScope();

        // SQLite cannot ALTER TABLE ADD a standalone foreign key (and enforces no
        // FKs by default anyway). LOOSE mode makes the generator skip those
        // unsupported statements on SQLite, while the same agnostic migration
        // still creates real, enforced FKs on Postgres/SQL Server.
        if (scope.ServiceProvider.GetRequiredService<IMigrationGenerator>() is GenericGenerator generator)
        {
            generator.CompatibilityMode = CompatibilityMode.LOOSE;
        }

        scope.ServiceProvider.GetRequiredService<IMigrationRunner>().MigrateUp();
    }
}
