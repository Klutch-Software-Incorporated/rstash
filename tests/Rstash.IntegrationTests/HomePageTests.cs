using System.Net;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

public sealed class HomePageTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Home_WhenSignedOut_RedirectsToLogin()
    {
        // An account exists (so the setup guard is satisfied), but the request is
        // unauthenticated — the dashboard redirects signed-out visitors to /login.
        await SeedAdminAsync();

        var client = factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });
        var response = await client.GetAsync("/");

        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
        Assert.Contains("/login", response.Headers.Location?.OriginalString);
    }

    private async Task SeedAdminAsync()
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        if (await users.FindByNameAsync("admin") is not null)
        {
            return;
        }

        var result = await users.CreateAsync(
            new ApplicationUser
            {
                UserName = "admin",
                CreatedAt = DateTimeOffset.UtcNow,
                IsAdmin = true,
                Approved = true,
            },
            "Admin!12345");

        Assert.True(result.Succeeded);
    }
}
