namespace Rstash.IntegrationTests;

public sealed class HomePageTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Home_RendersWelcomeFromSettings()
    {
        var client = factory.CreateClient();

        var html = await client.GetStringAsync("/");

        // Server-prerendered Blazor content, with the site name/subtitle pulled
        // from the settings snapshot.
        Assert.Contains("Welcome to rstash", html);
        Assert.Contains("A personal remoteStorage server.", html);
    }
}
