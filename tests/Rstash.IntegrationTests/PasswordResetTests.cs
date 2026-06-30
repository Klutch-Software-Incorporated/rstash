using System.Buffers.Text;
using System.Text;
using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

public sealed class PasswordResetTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task ResetPassword_WithValidToken_ChangesPassword()
    {
        const string username = "resetuser";

        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            await users.CreateAsync(
                new ApplicationUser { UserName = username, Email = "r@example.com", CreatedAt = DateTimeOffset.UtcNow, Approved = true },
                "OldPass!123");
        }

        string token;
        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            var user = await users.FindByNameAsync(username);
            var raw = await users.GeneratePasswordResetTokenAsync(user!);
            token = Base64Url.EncodeToString(Encoding.UTF8.GetBytes(raw));
        }

        var client = factory.CreateClient();
        var url = $"/reset-password?user={username}&token={token}";
        var antiforgery = FormHelpers.AntiforgeryToken(await client.GetStringAsync(url));

        var response = await client.PostAsync(url, new FormUrlEncodedContent(new Dictionary<string, string>
        {
            ["_handler"] = "reset",
            ["__RequestVerificationToken"] = antiforgery,
            ["Input.Password"] = "NewPass!456",
        }));
        response.EnsureSuccessStatusCode();

        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            var user = await users.FindByNameAsync(username);
            Assert.True(await users.CheckPasswordAsync(user!, "NewPass!456"));
        }
    }
}
