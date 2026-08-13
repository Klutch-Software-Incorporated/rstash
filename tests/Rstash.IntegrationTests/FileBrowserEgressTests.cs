using System.Net;
using System.Net.Http.Headers;
using System.Text;
using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Storage;

namespace Rstash.IntegrationTests;

/// <summary>
/// The file browser hands out the same bytes as the storage API, so it has to be
/// metered on the same terms. It previously was not: downloads through the web UI
/// spent no allowance and moved no meter.
/// </summary>
public sealed class FileBrowserEgressTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    private const string Password = "Sup3r!secret";

    [Fact]
    public async Task BrowserDownload_ChargesEgressToTheOwner()
    {
        var body = new string('x', 4096);
        var user = await SeedUserAsync("meter", "/documents/big.txt", body);
        var client = await SignedInClientAsync("meter");

        var before = await UsedAsync(user.Id);

        var download = await client.GetAsync("/files/download/documents/big.txt");

        Assert.Equal(HttpStatusCode.OK, download.StatusCode);
        Assert.Equal(body, await download.Content.ReadAsStringAsync());

        // Read through the tracker rather than the table: increments are buffered in
        // memory and flushed on a ten-second timer, and GetUsedAsync counts both.
        var after = await UsedAsync(user.Id);
        Assert.Equal(before + body.Length, after);
    }

    [Fact]
    public async Task BrowserDownload_IsRefusedOnceTheAllowanceIsSpent()
    {
        var body = new string('y', 2048);
        var user = await SeedUserAsync("capped", "/documents/big.txt", body);

        // An allowance smaller than the document: the first download cannot fit.
        await SetEgressQuotaAsync(user.Id, 512);

        var client = await SignedInClientAsync("capped");
        var download = await client.GetAsync("/files/download/documents/big.txt");

        Assert.Equal(HttpStatusCode.TooManyRequests, download.StatusCode);
        Assert.NotNull(download.Headers.RetryAfter);

        // And nothing was charged for the refusal.
        Assert.Equal(0, await UsedAsync(user.Id));
    }

    private async Task<long> UsedAsync(long userId) =>
        await factory.Services.GetRequiredService<EgressTracker>().GetUsedAsync(userId);

    private async Task<ApplicationUser> SeedUserAsync(string username, string path, string content)
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var created = await users.CreateAsync(
            new ApplicationUser { UserName = username, CreatedAt = DateTimeOffset.UtcNow, Approved = true },
            Password);
        Assert.True(created.Succeeded);

        var user = await users.FindByNameAsync(username);
        Assert.NotNull(user);

        // Put the document in through the storage API's own service, so the fixture
        // does not depend on the endpoint under test.
        var storage = factory.Services.GetRequiredService<RemoteStorageService>();
        using var stream = new MemoryStream(Encoding.UTF8.GetBytes(content));
        await storage.PutDocumentAsync(user!.Id, path, stream, "text/plain", new StorageConditions());

        return user;
    }

    private async Task SetEgressQuotaAsync(long userId, long quota)
    {
        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();
        var row = await db.Users.SingleAsync(u => u.Id == userId);
        row.EgressQuota = quota;
        await db.SaveChangesAsync();
    }

    private async Task<HttpClient> SignedInClientAsync(string username)
    {
        var client = factory.CreateClient();
        var response = await FormHelpers.SignInAsync(client, username, Password);
        response.EnsureSuccessStatusCode();
        return client;
    }
}
