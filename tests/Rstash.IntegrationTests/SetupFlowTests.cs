using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>End-to-end: the setup form actually creates the first admin.</summary>
public sealed class SetupFlowTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Setup_CreatesAdmin_AndCompletesSetup()
    {
        var client = factory.CreateClient(); // follows redirects, keeps cookies

        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));

        var form = new FormUrlEncodedContent(new Dictionary<string, string>
        {
            ["_handler"] = "setup",
            ["__RequestVerificationToken"] = token,
            ["Input.Username"] = "root",
            ["Input.Email"] = "root@example.com",
            ["Input.Password"] = "Sup3r!secret",
        });

        var response = await client.PostAsync("/setup", form);
        response.EnsureSuccessStatusCode(); // redirected to / which now renders

        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        var created = await users.FindByNameAsync("root");

        Assert.NotNull(created);
        Assert.True(created.IsAdmin);
        Assert.True(created.Approved);
    }
}
