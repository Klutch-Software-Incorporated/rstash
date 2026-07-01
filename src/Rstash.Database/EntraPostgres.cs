using Azure.Core;
using Azure.Identity;
using Npgsql;

namespace Rstash.Database;

/// <summary>
/// Azure Entra ID (managed-identity) auth for Azure Database for PostgreSQL,
/// selected by the <c>Auth=Entra</c> DSN flag (see <see cref="PostgresDsn"/>). Ports
/// <c>legacy/internal/db/entra_postgres.go</c>: the Postgres password is a
/// short-lived AAD access token rather than a static secret. The token is fetched via
/// <see cref="DefaultAzureCredential"/> (env service-principal → managed identity →
/// Azure CLI → …), so the same code works locally and on the hosted deployment.
/// </summary>
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
    /// periodically-refreshed Entra access token. The data source owns the connection
    /// pool and its refresh timer; it is <b>app-owned</b> (EF does not dispose a data
    /// source passed to <c>UseNpgsql</c>), which is safe here because the opener runs
    /// once at options-build time (singleton context factory / singleton blob store),
    /// never per <c>DbContext</c>. The credential is only invoked when a token is first
    /// needed (first connection open), so building options never blocks on Azure.
    /// </summary>
    internal static NpgsqlDataSource BuildDataSource(string connectionString)
    {
        // Construct the credential lazily on first token fetch — never at option-build
        // time — so building EF options never touches Azure. This keeps non-Azure
        // environments (and misconfigured credential chains) from failing at startup;
        // they surface at connection time instead (e.g. via `rstash check`).
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
