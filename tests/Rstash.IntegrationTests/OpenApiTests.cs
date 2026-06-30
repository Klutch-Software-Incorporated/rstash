using System.Net;

namespace Rstash.IntegrationTests;

public sealed class OpenApiTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task OpenApiDocument_IsServed()
    {
        var response = await factory.CreateClient().GetAsync("/openapi/v1.json");

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Contains("openapi", await response.Content.ReadAsStringAsync());
    }
}
