using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

public sealed class IdentityTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task UserManager_CreatesAndFindsUser()
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var user = new ApplicationUser
        {
            UserName = "alice",
            Email = "alice@example.com",
            CreatedAt = DateTimeOffset.UtcNow,
            IsAdmin = true,
            Approved = true,
        };

        var created = await users.CreateAsync(user, "Sup3r!secret");
        Assert.True(created.Succeeded, string.Join("; ", created.Errors.Select(e => e.Description)));

        var found = await users.FindByNameAsync("alice");
        Assert.NotNull(found);
        Assert.True(found.IsAdmin);
        Assert.True(await users.CheckPasswordAsync(found, "Sup3r!secret"));
    }
}
