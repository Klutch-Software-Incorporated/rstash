using System.Web;

namespace Rstash.Storage;

/// <summary>
/// A parsed Azure Blob DSN of the form
/// <c>azureblob://{account}/{container}[/{prefix}][?tls=bool&amp;endpoint=host&amp;key=…&amp;sas=…]</c>.
///
/// Authentication follows what the DSN carries: a <c>key</c> selects shared-key
/// auth, a <c>sas</c> selects SAS-token auth, and when neither is present we
/// fall back to <c>DefaultAzureCredential</c> (managed identity on Azure,
/// developer credentials locally). That default is the production path — the
/// deployed app sets just <c>azureblob://{account}/{container}</c> and relies on
/// its managed identity.
/// </summary>
public readonly record struct AzureBlobDsn(
    string Account,
    string Container,
    string Prefix,
    string? Key,
    string? Sas,
    string Endpoint,
    bool UseTls)
{
    public const string Scheme = "azureblob";

    public static AzureBlobDsn Parse(string dsn)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(dsn);

        if (!Uri.TryCreate(dsn, UriKind.Absolute, out var uri)
            || !string.Equals(uri.Scheme, Scheme, StringComparison.Ordinal))
        {
            throw new FormatException(
                $"azureblob DSN must look like \"azureblob://account/container\": {dsn}");
        }

        var account = uri.Host;
        if (string.IsNullOrEmpty(account))
        {
            throw new FormatException($"azureblob DSN is missing the storage account: {dsn}");
        }

        var segments = uri.AbsolutePath.Split('/', StringSplitOptions.RemoveEmptyEntries);
        if (segments.Length == 0)
        {
            throw new FormatException($"azureblob DSN is missing the container name: {dsn}");
        }

        var query = HttpUtility.ParseQueryString(uri.Query);
        var endpoint = query["endpoint"];
        if (string.IsNullOrWhiteSpace(endpoint))
        {
            endpoint = $"{account}.blob.core.windows.net";
        }

        return new AzureBlobDsn(
            Account: account,
            Container: segments[0],
            Prefix: string.Join('/', segments.Skip(1)),
            Key: NullIfEmpty(query["key"]),
            Sas: NullIfEmpty(query["sas"]),
            Endpoint: endpoint,
            UseTls: ParseTls(query["tls"], dsn));
    }

    /// <summary>
    /// The container endpoint the Azure SDK talks to. TLS uses host-style
    /// <c>https://{endpoint}/{container}</c>; plaintext (Azurite) uses the
    /// path-style <c>http://{endpoint}/{account}/{container}</c>.
    /// </summary>
    public Uri ContainerUri => UseTls
        ? new Uri($"https://{Endpoint}/{Container}")
        : new Uri($"http://{Endpoint}/{Account}/{Container}");

    private static bool ParseTls(string? value, string dsn)
    {
        if (string.IsNullOrEmpty(value))
        {
            return true;
        }

        return bool.TryParse(value, out var tls)
            ? tls
            : throw new FormatException($"azureblob DSN has an invalid tls value \"{value}\": {dsn}");
    }

    private static string? NullIfEmpty(string? value) => string.IsNullOrEmpty(value) ? null : value;
}
