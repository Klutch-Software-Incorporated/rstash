using System.Net;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

public sealed class AdminTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task AdminSettings_Anonymous_RedirectsToLogin()
    {
        // A user must exist so the first-run setup guard is satisfied.
        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            if (await users.FindByNameAsync("seed") is null)
            {
                await users.CreateAsync(
                    new ApplicationUser { UserName = "seed", CreatedAt = DateTimeOffset.UtcNow, Approved = true },
                    "Sup3r!secret");
            }
        }

        // rstash authenticates as an OpenID Connect relying party, so an anonymous
        // request no longer jumps straight to a password form: it challenges the
        // provider, which finds no session and challenges Identity in turn. Following
        // the chain asserts the user-visible outcome is unchanged.
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = true });

        var response = await client.GetAsync("/admin/settings");

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Contains("/login", response.RequestMessage!.RequestUri!.PathAndQuery);
    }
}
