using Microsoft.EntityFrameworkCore;
using Npgsql;
using Rstash.Database;

namespace Rstash.Core.Tests;

/// <summary>
/// Pure-unit coverage of <see cref="PostgresDsn.Parse"/> and the Entra opener path —
/// no PostgreSQL server and no Azure required (the Entra flag is only <em>detected</em>
/// here; the token is fetched lazily on first connection open).
/// </summary>
public class PostgresDsnTests
{
    [Fact]
    public void Parse_NativeString_PassesThroughWithoutEntra()
    {
        var (connectionString, useEntra) = PostgresDsn.Parse(
            "Host=localhost;Database=rstash;Username=me;Password=secret");

        Assert.False(useEntra);
        var csb = new NpgsqlConnectionStringBuilder(connectionString);
        Assert.Equal("localhost", csb.Host);
        Assert.Equal("rstash", csb.Database);
        Assert.Equal("me", csb.Username);
    }

    [Fact]
    public void Parse_AuthEntra_IsDetectedAndStripped()
    {
        var (connectionString, useEntra) = PostgresDsn.Parse(
            "Host=db.postgres.database.azure.com;Database=rstash;Username=alice;Ssl Mode=Require;Auth=Entra");

        Assert.True(useEntra);

        var csb = new NpgsqlConnectionStringBuilder(connectionString);
        Assert.Equal("alice", csb.Username); // preserved — the token supplies only the password.
        Assert.Equal("rstash", csb.Database);
        Assert.Equal(SslMode.Require, csb.SslMode);

        // The pseudo-keyword must not leak into the driver-facing string.
        Assert.DoesNotContain("Auth", connectionString, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("auth=entra")]
    [InlineData("AUTH=ENTRA")]
    [InlineData("Auth = Entra")]
    public void Parse_AuthEntra_IsCaseAndWhitespaceInsensitive(string segment)
    {
        var (_, useEntra) = PostgresDsn.Parse($"Host=localhost;Database=rstash;{segment}");

        Assert.True(useEntra);
    }

    [Theory]
    [InlineData("postgres://user:pass@host/db")]
    [InlineData("postgresql://user@host/db")]
    public void Parse_UrlForm_ThrowsActionableError(string url)
    {
        var ex = Assert.Throws<ArgumentException>(() => PostgresDsn.Parse(url));

        Assert.Contains("native", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Parse_UnsupportedAuthValue_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(
            () => PostgresDsn.Parse("Host=localhost;Database=rstash;Auth=Kerberos"));

        Assert.Contains("Entra", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void UseRstashDatabase_EntraDsn_BuildsOptionsWithoutContactingAzure()
    {
        // Option-building must not eagerly fetch a token — the periodic password
        // provider defers that to first connection open. So this succeeds even with no
        // Azure credentials available in the environment.
        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase("postgres:Host=localhost;Database=rstash;Username=alice;Auth=Entra")
            .Options;

        Assert.NotNull(options);
    }
}
