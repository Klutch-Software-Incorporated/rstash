using System.Net;

namespace Rstash.IntegrationTests;

public sealed class WebFingerTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task WebFinger_ReturnsStorageDescriptor()
    {
        var response = await factory.CreateClient()
            .GetAsync("/.well-known/webfinger?resource=acct:Alice@example.com");

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("application/jrd+json", response.Content.Headers.ContentType?.MediaType);

        var body = await response.Content.ReadAsStringAsync();
        Assert.Contains("acct:alice@example.com", body);                 // canonical lowercase
        Assert.Contains("http://localhost:8080/storage/alice", body);
        Assert.Contains("draft-dejong-remotestorage-26", body);
        Assert.Contains("/oauth/authorize", body);
        Assert.Contains("/oauth/token", body);
    }

    [Fact]
    public async Task WebFinger_MissingResource_Returns400()
    {
        var response = await factory.CreateClient().GetAsync("/.well-known/webfinger");

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
    }
}
