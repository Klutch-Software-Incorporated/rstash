using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.Core.Tests;

/// <summary>
/// Token issuance and refresh rotation against a real SQLite database, because
/// both bugs these cover are about what actually lands in — and leaves — the
/// oauth_tokens table.
/// </summary>
public class TokenStoreTests
{
    /// <summary>
    /// The implicit flow returns its token in a URL fragment, so it must not be
    /// handed a refresh secret to put there. Suppression used to be inferred from
    /// a null refresh lifetime, which also means "never expires" — so every
    /// implicit grant stored a refresh token that worked forever.
    /// </summary>
    [Fact]
    public async Task CreateAsync_WithoutRefreshToken_StoresNoRefreshSecret()
    {
        await WithStoreAsync(async (store, _) =>
        {
            var token = await store.CreateAsync(
                userId: 1, "https://implicit.example.com", "*:rw",
                lifetime: TimeSpan.FromHours(1), withRefreshToken: false);

            Assert.Null(token.RefreshToken);
            Assert.Null(token.RefreshExpiresAt);
        });
    }

    [Fact]
    public async Task CreateAsync_WithRefreshToken_StoresARedeemableSecret()
    {
        await WithStoreAsync(async (store, _) =>
        {
            var token = await store.CreateAsync(
                userId: 1, "https://app.example.com", "*:rw",
                lifetime: TimeSpan.FromHours(1), withRefreshToken: true, TimeSpan.FromDays(90));

            Assert.NotNull(token.RefreshToken);
            Assert.NotNull(token.RefreshExpiresAt);
            Assert.NotNull(await store.FindByRefreshTokenAsync(token.RefreshToken!));
        });
    }

    /// <summary>
    /// A refresh token with no expiry is legitimate — refresh_token_lifetime=0 is
    /// documented as "no expiry" — so a null lifetime must still issue one.
    /// </summary>
    [Fact]
    public async Task CreateAsync_WithRefreshTokenAndNoExpiry_StillIssuesOne()
    {
        await WithStoreAsync(async (store, _) =>
        {
            var token = await store.CreateAsync(
                userId: 1, "https://app.example.com", "*:rw",
                lifetime: null, withRefreshToken: true, refreshLifetime: null);

            Assert.NotNull(token.RefreshToken);
            Assert.Null(token.RefreshExpiresAt);
        });
    }

    /// <summary>
    /// Two requests racing the same refresh token must produce exactly one live
    /// token family. Rotation claims the old row by deleting it; if the delete's
    /// row count is ignored, both callers "succeed" and an attacker who races the
    /// real client keeps a working pair — the precise replay rotation exists to
    /// prevent.
    /// </summary>
    [Fact]
    public async Task RefreshAsync_ConcurrentRedemptions_OnlyOneWins()
    {
        await WithStoreAsync(async (store, _) =>
        {
            var original = await store.CreateAsync(
                userId: 1, "https://app.example.com", "*:rw",
                lifetime: TimeSpan.FromHours(1), withRefreshToken: true, TimeSpan.FromDays(90));

            var secret = original.RefreshToken!;

            var results = await Task.WhenAll(
                Enumerable.Range(0, 8).Select(_ => Task.Run(() =>
                    store.RefreshAsync(secret, TimeSpan.FromHours(1), TimeSpan.FromDays(90)))));

            var winners = results.Where(t => t is not null).ToList();
            Assert.Single(winners);

            // And the losers left nothing behind: the winner's family is the only one.
            Assert.Null(await store.ValidateAsync(original.Token));
            Assert.NotNull(await store.ValidateAsync(winners[0]!.Token));
        });
    }

    [Fact]
    public async Task RefreshAsync_SequentialReplay_IsRejected()
    {
        await WithStoreAsync(async (store, _) =>
        {
            var original = await store.CreateAsync(
                userId: 1, "https://app.example.com", "*:rw",
                lifetime: TimeSpan.FromHours(1), withRefreshToken: true, TimeSpan.FromDays(90));

            var secret = original.RefreshToken!;

            Assert.NotNull(await store.RefreshAsync(secret, TimeSpan.FromHours(1), TimeSpan.FromDays(90)));
            Assert.Null(await store.RefreshAsync(secret, TimeSpan.FromHours(1), TimeSpan.FromDays(90)));
        });
    }

    private static async Task WithStoreAsync(Func<TokenStore, string, Task> body)
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-tokens-{Guid.NewGuid():N}.sqlite");
        var dsn = $"sqlite:{path}";

        try
        {
            SchemaMigrator.MigrateUp(dsn);
            await body(new TokenStore(new SqliteContextFactory(dsn)), dsn);
        }
        finally
        {
            SqliteConnection.ClearAllPools();
            File.Delete(path);
        }
    }

    private sealed class SqliteContextFactory(string dsn) : IDbContextFactory<RstashDbContext>
    {
        public RstashDbContext CreateDbContext() =>
            new(new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options);
    }
}
