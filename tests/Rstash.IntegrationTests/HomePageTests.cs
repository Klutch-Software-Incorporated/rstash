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
        // unauthenticated.
        await SeedAdminAsync();

        // The dashboard no longer redirects to /login itself: it carries [Authorize],
        // so it challenges OpenID Connect, the provider finds no session and challenges
        // Identity, and *that* is what serves the password form. Asserting on the first
        // Location would only pin down the first hop of three; what matters to the
        // person is where they end up, so follow the chain.
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
            MaxAutomaticRedirections = 12,
        });

        var response = await client.GetAsync("/");

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Contains("/login", response.RequestMessage!.RequestUri!.PathAndQuery);
        Assert.Contains("__RequestVerificationToken", await response.Content.ReadAsStringAsync());
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
