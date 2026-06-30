using Rstash.Database;

namespace Rstash.Core.Tests;

public class DatabaseDsnTests
{
    [Theory]
    [InlineData("sqlite:rstash.sqlite", Dialect.Sqlite, "rstash.sqlite")]
    [InlineData("rstash.sqlite", Dialect.Sqlite, "rstash.sqlite")]
    [InlineData("mssql:Server=.;Database=rstash", Dialect.SqlServer, "Server=.;Database=rstash")]
    public void Parse_ExtractsSchemeAndConnection(string dsn, Dialect dialect, string connection)
    {
        var parsed = DatabaseDsn.Parse(dsn);

        Assert.Equal(dialect, parsed.Dialect);
        Assert.Equal(connection, parsed.ConnectionString);
    }

    [Fact]
    public void Parse_PostgresUrl_KeepsSchemeIntact()
    {
        var parsed = DatabaseDsn.Parse("postgres://user:pass@host/db");

        Assert.Equal(Dialect.Postgres, parsed.Dialect);
        Assert.Equal("postgres://user:pass@host/db", parsed.ConnectionString);
    }

    [Fact]
    public void Parse_PostgresKeyValue_StripsScheme()
    {
        var parsed = DatabaseDsn.Parse("postgres:host=localhost dbname=rstash");

        Assert.Equal(Dialect.Postgres, parsed.Dialect);
        Assert.Equal("host=localhost dbname=rstash", parsed.ConnectionString);
    }
}
