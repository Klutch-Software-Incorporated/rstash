using Azure.Core;
using Azure.Identity;
using Npgsql;

namespace Rstash.Database;

/// <summary>
/// Azure Entra ID (managed-identity) auth for Azure Database for PostgreSQL,
/// selected by the <c>Auth=Entra</c> DSN flag (see <see cref="PostgresDsn"/>).
/// </summary>
/// <remarks>
/// The Postgres password is a short-lived Entra access token rather than a static
/// secret, obtained from <see cref="DefaultAzureCredential"/>, which walks the standard
/// credential chain (environment service principal, managed identity, Azure CLI), so
/// the same code runs locally and on the hosted deployment.
/// </remarks>
internal static class EntraPostgres
{
    /// <summary>
    /// The AAD scope for the Azure "OSS RDBMS" service family. Postgres and MySQL
    /// Flexible Server share it, so a future MySQL Entra arm can reuse this class;
    /// SQL Server uses a different mechanism (SqlClient's <c>Active Directory
    /// Default</c>) and does not.
    /// </summary>
    internal const string Scope = "https://ossrdbms-aad.database.windows.net/.default";

    private static readonly TokenRequestContext TokenRequest = new([Scope]);

    /// <summary>
    /// Builds a long-lived <see cref="NpgsqlDataSource"/> whose password is a
    /// periodically-refreshed Entra access token.
    /// </summary>
    /// <remarks>
    /// The data source owns the connection pool and its refresh timer, and is app-owned:
    /// EF does not dispose one passed to <c>UseNpgsql</c>. That is safe because the opener
    /// runs once at options-build time (singleton context factory / singleton blob store),
    /// never per <c>DbContext</c>.
    /// </remarks>
    internal static NpgsqlDataSource BuildDataSource(string connectionString)
    {
        // Build the credential lazily, inside the token callback, so option-building
        // never touches Azure. A non-Azure host or a broken credential chain then fails
        // at first connection (e.g. via `rstash check`) rather than at startup.
        var credential = new Lazy<DefaultAzureCredential>(() => new DefaultAzureCredential());
        var builder = new NpgsqlDataSourceBuilder(connectionString);
        builder.UsePeriodicPasswordProvider(
            async (_, ct) => (await credential.Value.GetTokenAsync(TokenRequest, ct)).Token,
            // Azure AD tokens live ~60–90 min; refresh comfortably before expiry, and
            // retry quickly after a transient failure.
            successRefreshInterval: TimeSpan.FromMinutes(55),
            failureRefreshInterval: TimeSpan.FromSeconds(10));
        return builder.Build();
    }

    /// <summary>
    /// Fetches a single access token synchronously. Used by the one-shot schema
    /// migration (which has no data-source seam and completes well within a token's
    /// lifetime) — see <see cref="SchemaMigrator"/>.
    /// </summary>
    internal static string FetchToken()
        => new DefaultAzureCredential().GetToken(TokenRequest, CancellationToken.None).Token;
}
