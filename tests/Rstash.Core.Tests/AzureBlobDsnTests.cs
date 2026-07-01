using Rstash.Storage;

namespace Rstash.Core.Tests;

public class AzureBlobDsnTests
{
    [Fact]
    public void Parse_ProductionForm_SelectsManagedIdentity()
    {
        var dsn = AzureBlobDsn.Parse("azureblob://rstprdsteus2001/rstash");

        Assert.Equal("rstprdsteus2001", dsn.Account);
        Assert.Equal("rstash", dsn.Container);
        Assert.Equal("", dsn.Prefix);
        Assert.Null(dsn.Key);
        Assert.Null(dsn.Sas);
        Assert.Equal("rstprdsteus2001.blob.core.windows.net", dsn.Endpoint);
        Assert.True(dsn.UseTls);
    }

    [Fact]
    public void Parse_KeepsExtraPathSegmentsAsPrefix()
    {
        var dsn = AzureBlobDsn.Parse("azureblob://acct/container/team/photos");

        Assert.Equal("container", dsn.Container);
        Assert.Equal("team/photos", dsn.Prefix);
    }

    [Fact]
    public void Parse_ReadsKeyEndpointAndTls()
    {
        var dsn = AzureBlobDsn.Parse(
            "azureblob://devstoreaccount1/rstash?endpoint=127.0.0.1:10000&tls=false&key=c2VjcmV0");

        Assert.Equal("c2VjcmV0", dsn.Key);
        Assert.Equal("127.0.0.1:10000", dsn.Endpoint);
        Assert.False(dsn.UseTls);
    }

    [Fact]
    public void Parse_ReadsSasToken()
    {
        var dsn = AzureBlobDsn.Parse("azureblob://acct/rstash?sas=sv%3D2022-11-02%26sig%3Dabc");

        Assert.Equal("sv=2022-11-02&sig=abc", dsn.Sas);
        Assert.Null(dsn.Key);
    }

    [Fact]
    public void ContainerUri_Tls_IsHostStyle()
    {
        var dsn = AzureBlobDsn.Parse("azureblob://acct/rstash");

        Assert.Equal("https://acct.blob.core.windows.net/rstash", dsn.ContainerUri.ToString());
    }

    [Fact]
    public void ContainerUri_NoTls_IsAzuritePathStyle()
    {
        var dsn = AzureBlobDsn.Parse("azureblob://devstoreaccount1/rstash?endpoint=127.0.0.1:10000&tls=false");

        Assert.Equal("http://127.0.0.1:10000/devstoreaccount1/rstash", dsn.ContainerUri.ToString());
    }

    [Theory]
    [InlineData("azureblob://acct", "container")]          // no container segment
    [InlineData("azureblob:///rstash", "account")]         // no host/account
    [InlineData("s3://acct/rstash", "azureblob://")]       // wrong scheme
    [InlineData("azureblob://acct/rstash?tls=maybe", "tls")] // bad bool
    public void Parse_Invalid_Throws(string dsn, string messageFragment)
    {
        var ex = Assert.Throws<FormatException>(() => AzureBlobDsn.Parse(dsn));
        Assert.Contains(messageFragment, ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Parse_NullOrWhitespace_Throws()
    {
        Assert.Throws<ArgumentException>(() => AzureBlobDsn.Parse(""));
        Assert.Throws<ArgumentException>(() => AzureBlobDsn.Parse("   "));
    }
}
