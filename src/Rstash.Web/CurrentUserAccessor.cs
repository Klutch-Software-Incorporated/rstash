using System.Security.Claims;
using Microsoft.AspNetCore.Identity;
using Rstash.Database;

namespace Rstash.Web;

/// <summary>
/// Loads the signed-in <see cref="ApplicationUser"/> once per request/circuit and
/// memoizes the in-flight task, so multiple components that need the current user
/// during one render (e.g. the layout plus the page) share a single query instead of
/// each issuing its own on the shared scoped <c>RstashDbContext</c>.
/// <para>
/// Two concurrent <see cref="UserManager{TUser}"/> reads on that shared context throw
/// EF Core's "A second operation was started on this context instance" — a race that
/// stayed hidden on SQLite (sub-millisecond queries never overlapped) but is reliably
/// tripped by Postgres's network latency. Registered scoped so the cache lives for the
/// request/circuit.
/// </para>
/// </summary>
public sealed class CurrentUserAccessor(UserManager<ApplicationUser> users)
{
    private Task<ApplicationUser?>? _cached;

    /// <summary>
    /// Returns the user for <paramref name="principal"/>, or <c>null</c> when it is
    /// <c>null</c> or unauthenticated. The first authenticated call starts the load;
    /// later calls in the same scope await the same task (one query, no concurrency).
    /// </summary>
    public Task<ApplicationUser?> GetAsync(ClaimsPrincipal? principal)
    {
        if (principal?.Identity?.IsAuthenticated != true)
        {
            return Task.FromResult<ApplicationUser?>(null);
        }

        return _cached ??= users.GetUserAsync(principal);
    }
}
