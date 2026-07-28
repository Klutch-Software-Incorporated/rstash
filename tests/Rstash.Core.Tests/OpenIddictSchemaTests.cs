using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using OpenIddict.EntityFrameworkCore.Models;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// FluentMigrator owns the DDL for OpenIddict's stores, but OpenIddict queries them
/// through EF. Nothing in the build forces the hand-written migration and the
/// package's EF model to agree, and a mismatch surfaces at runtime as a failed login
/// rather than a failed build — so it is asserted here.
/// </summary>
public sealed class OpenIddictSchemaTests : IDisposable
{
    private readonly string _path =
        Path.Combine(Path.GetTempPath(), $"rstash-oi-{Guid.NewGuid():N}.sqlite");

    public void Dispose()
    {
        SqliteConnection.ClearAllPools();
        foreach (var p in new[] { _path, _path + "-wal", _path + "-shm" })
        {
            try
            {
                File.Delete(p);
            }
            catch (IOException)
            {
            }
        }
    }

    [Fact]
    public void MigratedSchema_HasTheFourStoresAndTheirIndexes()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        using var connection = new SqliteConnection($"Data Source={_path}");
        connection.Open();

        var tables = Query(connection, "SELECT name FROM sqlite_master WHERE type = 'table';");
        foreach (var table in new[]
                 {
                     "OpenIddictApplications", "OpenIddictAuthorizations",
                     "OpenIddictScopes", "OpenIddictTokens",
                 })
        {
            Assert.Contains(table, tables);
        }

        var uniqueIndexes = Query(
            connection,
            "SELECT name FROM sqlite_master WHERE type = 'index' AND sql LIKE '%UNIQUE%';");
        foreach (var index in new[]
                 {
                     "IX_OpenIddictApplications_ClientId",
                     "IX_OpenIddictScopes_Name",
                     "IX_OpenIddictTokens_ReferenceId",
                 })
        {
            Assert.Contains(index, uniqueIndexes);
        }
    }

    [Fact]
    public async Task EfModel_RoundTripsAgainstMigratedSchema()
    {
        SchemaMigrator.MigrateUp($"sqlite:{_path}");

        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase($"sqlite:{_path}")
            .Options;

        var authorizationId = Guid.NewGuid().ToString();
        var created = new DateTime(2026, 7, 28, 12, 0, 0, DateTimeKind.Utc);

        await using (var ctx = new RstashDbContext(options))
        {
            // One row per table, walking the chain application -> authorization ->
            // token, which is the shape a real authorization-code login produces. The
            // links go through navigation properties: OpenIddict keeps the foreign-key
            // columns as EF shadow properties, so there is no ApplicationId to assign.
            var application = new OpenIddictEntityFrameworkCoreApplication
            {
                Id = Guid.NewGuid().ToString(),
                ClientId = "rstash-web",
                ClientType = "confidential",
                DisplayName = "rstash",
            };

            var authorization = new OpenIddictEntityFrameworkCoreAuthorization
            {
                Id = authorizationId,
                Application = application,
                Status = "valid",
                Subject = "42",
                Type = "permanent",
                CreationDate = created,
            };

            ctx.Add(application);
            ctx.Add(authorization);
            ctx.Add(new OpenIddictEntityFrameworkCoreToken
            {
                Id = Guid.NewGuid().ToString(),
                Application = application,
                Authorization = authorization,
                ReferenceId = "ref-1",
                Status = "valid",
                Subject = "42",
                Type = "access_token",
                CreationDate = created,
                ExpirationDate = created.AddHours(1),
            });

            ctx.Add(new OpenIddictEntityFrameworkCoreScope
            {
                Id = Guid.NewGuid().ToString(),
                Name = "rstash.entitlements",
            });

            await ctx.SaveChangesAsync();
        }

        await using (var ctx = new RstashDbContext(options))
        {
            Assert.Equal(
                "rstash-web",
                (await ctx.Set<OpenIddictEntityFrameworkCoreApplication>().SingleAsync()).ClientId);

            var token = await ctx.Set<OpenIddictEntityFrameworkCoreToken>()
                .Include(t => t.Authorization)
                .SingleAsync();
            Assert.Equal(authorizationId, token.Authorization!.Id);

            // Timestamps are the column type most likely to drift: OpenIddict uses
            // DateTime where the rest of rstash uses DateTimeOffset.
            Assert.Equal(created.AddHours(1), token.ExpirationDate);

            Assert.Equal(
                "rstash.entitlements",
                (await ctx.Set<OpenIddictEntityFrameworkCoreScope>().SingleAsync()).Name);
        }
    }

    private static List<string> Query(SqliteConnection connection, string sql)
    {
        using var command = connection.CreateCommand();
        command.CommandText = sql;
        using var reader = command.ExecuteReader();
        var results = new List<string>();
        while (reader.Read())
        {
            results.Add(reader.GetString(0));
        }

        return results;
    }
}
