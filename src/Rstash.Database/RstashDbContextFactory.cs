using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Design;

namespace Rstash.Database;

/// <summary>
/// Design-time factory so <c>dotnet ef migrations</c> can build the context
/// without a running host. Uses a throwaway SQLite file purely for model
/// discovery; runtime configuration comes from the real DSN.
/// </summary>
internal sealed class RstashDbContextFactory : IDesignTimeDbContextFactory<RstashDbContext>
{
    public RstashDbContext CreateDbContext(string[] args)
    {
        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase("sqlite:rstash-design.sqlite")
            .Options;

        return new RstashDbContext(options);
    }
}
