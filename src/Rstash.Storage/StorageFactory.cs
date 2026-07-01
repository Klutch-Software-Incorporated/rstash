namespace Rstash.Storage;

/// <summary>
/// Opens a blob store from a DSN. The supported scheme list is the source of
/// truth shared with config validation. Encryption wrapping (RSTASH_BLOB_KEY)
/// and the S3/database backends are layered on in later chunks.
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
            // pick a provider). SQLite is wired; other providers throw until
            // their packages land.
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
