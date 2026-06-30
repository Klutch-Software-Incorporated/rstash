using System.Net;
using System.Net.Http.Headers;
using System.Text;
using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.IntegrationTests;

public sealed class StorageApiTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Put_Get_Head_Delete_Lifecycle()
    {
        var (user, token) = await SeedUserWithTokenAsync("alice", "*:rw");
        var client = factory.CreateClient();

        var put = await client.SendAsync(Authed(HttpMethod.Put, $"/storage/{user}/documents/notes.txt", token,
            new StringContent("hello", Encoding.UTF8, "text/plain")));
        Assert.Equal(HttpStatusCode.Created, put.StatusCode);
        Assert.NotNull(put.Headers.ETag);

        var get = await client.SendAsync(Authed(HttpMethod.Get, $"/storage/{user}/documents/notes.txt", token));
        Assert.Equal(HttpStatusCode.OK, get.StatusCode);
        Assert.Equal("hello", await get.Content.ReadAsStringAsync());
        Assert.Equal(put.Headers.ETag, get.Headers.ETag);

        var head = await client.SendAsync(Authed(HttpMethod.Head, $"/storage/{user}/documents/notes.txt", token));
        Assert.Equal(HttpStatusCode.OK, head.StatusCode);
        Assert.Equal(5, head.Content.Headers.ContentLength);

        var folder = await client.SendAsync(Authed(HttpMethod.Get, $"/storage/{user}/documents/", token));
        Assert.Equal(HttpStatusCode.OK, folder.StatusCode);
        Assert.Equal("application/ld+json", folder.Content.Headers.ContentType?.MediaType);
        var folderBody = await folder.Content.ReadAsStringAsync();
        Assert.Contains("notes.txt", folderBody);
        Assert.Contains("@context", folderBody);

        var del = await client.SendAsync(Authed(HttpMethod.Delete, $"/storage/{user}/documents/notes.txt", token));
        Assert.Equal(HttpStatusCode.OK, del.StatusCode);

        var after = await client.SendAsync(Authed(HttpMethod.Get, $"/storage/{user}/documents/notes.txt", token));
        Assert.Equal(HttpStatusCode.NotFound, after.StatusCode);
    }

    [Fact]
    public async Task MissingToken_Returns401()
    {
        var (user, _) = await SeedUserWithTokenAsync("bob", "*:rw");

        var response = await factory.CreateClient().GetAsync($"/storage/{user}/documents/x.txt");

        Assert.Equal(HttpStatusCode.Unauthorized, response.StatusCode);
    }

    [Fact]
    public async Task InsufficientScope_Returns403()
    {
        var (user, token) = await SeedUserWithTokenAsync("carol", "contacts:r");

        var response = await factory.CreateClient().SendAsync(
            Authed(HttpMethod.Put, $"/storage/{user}/photos/p.jpg", token, new StringContent("x")));

        Assert.Equal(HttpStatusCode.Forbidden, response.StatusCode);
    }

    [Fact]
    public async Task PublicDocument_ReadableWithoutAuth()
    {
        var (user, token) = await SeedUserWithTokenAsync("dave", "*:rw");
        var client = factory.CreateClient();

        await client.SendAsync(Authed(HttpMethod.Put, $"/storage/{user}/public/notes/pic.txt", token,
            new StringContent("pixels", Encoding.UTF8, "text/plain")));

        var response = await client.GetAsync($"/storage/{user}/public/notes/pic.txt"); // no auth header
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("pixels", await response.Content.ReadAsStringAsync());
    }

    [Fact]
    public async Task PublicWritesDisabled_RefusesProtocolWritesButKeepsReads()
    {
        var (user, token) = await SeedUserWithTokenAsync("frank", "*:rw");
        var client = factory.CreateClient();

        // Seed a public document while writes are still allowed.
        var seed = await client.SendAsync(Authed(HttpMethod.Put, $"/storage/{user}/public/site/index.html", token,
            new StringContent("<h1>hi</h1>", Encoding.UTF8, "text/html")));
        Assert.Equal(HttpStatusCode.Created, seed.StatusCode);

        var settings = factory.Services.GetRequiredService<SettingsService>();
        await settings.SetAsync("allow_public_writes", "disabled");
        try
        {
            // Writes and deletes under /public/ are refused...
            var put = await client.SendAsync(Authed(HttpMethod.Put, $"/storage/{user}/public/site/new.html", token,
                new StringContent("nope")));
            Assert.Equal(HttpStatusCode.Forbidden, put.StatusCode);

            var del = await client.SendAsync(
                Authed(HttpMethod.Delete, $"/storage/{user}/public/site/index.html", token));
            Assert.Equal(HttpStatusCode.Forbidden, del.StatusCode);

            // ...but public reads still work, and non-public writes are unaffected.
            var read = await client.GetAsync($"/storage/{user}/public/site/index.html");
            Assert.Equal(HttpStatusCode.OK, read.StatusCode);

            var privatePut = await client.SendAsync(Authed(HttpMethod.Put, $"/storage/{user}/notes/p.txt", token,
                new StringContent("ok")));
            Assert.Equal(HttpStatusCode.Created, privatePut.StatusCode);
        }
        finally
        {
            await settings.DeleteAsync("allow_public_writes");
        }
    }

    [Fact]
    public async Task ExceedingUserQuota_Returns507()
    {
        var (user, token) = await SeedUserWithTokenAsync("quotauser", "*:rw");

        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            var account = await users.FindByNameAsync(user);
            account!.StorageQuota = 10;
            await users.UpdateAsync(account);
        }

        var client = factory.CreateClient();

        var within = await client.SendAsync(
            Authed(HttpMethod.Put, $"/storage/{user}/a.txt", token, new StringContent("12345")));
        Assert.Equal(HttpStatusCode.Created, within.StatusCode);

        var exceeding = await client.SendAsync(
            Authed(HttpMethod.Put, $"/storage/{user}/b.txt", token, new StringContent("123456789012345678901")));
        Assert.Equal(HttpStatusCode.InsufficientStorage, exceeding.StatusCode);
    }

    [Fact]
    public async Task Put_RecordsAuditEntry()
    {
        var (user, token) = await SeedUserWithTokenAsync("audituser", "*:rw");

        await factory.CreateClient().SendAsync(
            Authed(HttpMethod.Put, $"/storage/{user}/x.txt", token, new StringContent("hi")));

        using var scope = factory.Services.CreateScope();
        var audit = scope.ServiceProvider.GetRequiredService<AuditService>();
        var recent = await audit.RecentAsync(50);

        Assert.Contains(recent, e => e.Action == "storage.put" && e.TargetId == "/x.txt");
    }

    [Fact]
    public async Task UnknownUser_Returns404()
    {
        var (_, token) = await SeedUserWithTokenAsync("erin", "*:rw");

        var response = await factory.CreateClient()
            .SendAsync(Authed(HttpMethod.Get, "/storage/nobody/x.txt", token));

        Assert.Equal(HttpStatusCode.NotFound, response.StatusCode);
    }

    private async Task<(string Username, string Token)> SeedUserWithTokenAsync(string username, string scopes)
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var user = await users.FindByNameAsync(username);
        if (user is null)
        {
            var created = await users.CreateAsync(
                new ApplicationUser { UserName = username, CreatedAt = DateTimeOffset.UtcNow, Approved = true },
                "Sup3r!secret");
            Assert.True(created.Succeeded);
            user = await users.FindByNameAsync(username);
        }

        var tokens = scope.ServiceProvider.GetRequiredService<TokenStore>();
        var token = await tokens.CreateAsync(user!.Id, "test-client", scopes, lifetime: null);
        return (username, token.Token);
    }

    private static HttpRequestMessage Authed(HttpMethod method, string url, string token, HttpContent? content = null)
    {
        var request = new HttpRequestMessage(method, url) { Content = content };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return request;
    }
}
