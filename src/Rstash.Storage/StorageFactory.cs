namespace Rstash.Storage;

/// <summary>
/// Opens a blob store from a DSN. The supported scheme list is the source of
/// truth shared with config validation.
/// <para>
/// The filesystem, database, and Azure Blob backends are wired; S3 is not. There
/// is deliberately no app-level encryption wrapper: an app-held key defends only
/// against a leaked storage credential without the app environment, and carries
/// an unrotatable lose-the-key-lose-everything footgun. Use a customer-managed
/// key in the storage account instead. See docs/PARITY-GAPS.md.
/// </para>
/// </summary>
public static class StorageFactory
{
    public static IReadOnlyList<string> SupportedSchemes { get; } =
        ["sqlite", "fs", "postgres", "mysql", "mssql", "s3", "azureblob"];

    public static bool IsSupportedScheme(string scheme) => SupportedSchemes.Contains(scheme);

    public static IStorage Open(string dsn)
    {
        var (scheme, spec) = SplitDsn(dsn);

        return scheme switch
        {
            "fs" => new FileSystemStorage(spec),
            // The database backend takes the full DSN (it parses the scheme to
            // pick a provider). SQLite and Postgres are wired; MySQL and SQL
            // Server throw until their packages land.
            "sqlite" or "postgres" or "mysql" or "mssql" => new DatabaseStorage(dsn),
            // Object stores take the full DSN and parse it themselves.
            "azureblob" => new AzureBlobStorage(dsn),
            "s3" => throw new NotSupportedException("s3 blob backend deferred (needs the AWS SDK)."),
            _ => throw new NotSupportedException(
                $"unsupported blob scheme \"{scheme}\" (supported: {string.Join(", ", SupportedSchemes)})"),
        };
    }

    private static (string Scheme, string Spec) SplitDsn(string dsn)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(dsn);

        var idx = dsn.IndexOf(':', StringComparison.Ordinal);
        return idx < 0
            ? ("fs", dsn) // bare path → filesystem
            : (dsn[..idx], dsn[(idx + 1)..]);
    }
}
