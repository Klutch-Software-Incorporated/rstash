using Npgsql;

namespace Rstash.Database;

/// <summary>
/// Parses the connection portion of a <c>postgres:</c> DSN into a native Npgsql
/// connection string and detects the rstash-specific <c>Auth=Entra</c> flag.
/// <para>
/// Unlike Go's <c>pgx</c> (the parity oracle in <c>legacy/internal/db</c>), Npgsql
/// does not accept <c>postgres://…</c> URLs — only its own semicolon-keyword form
/// (<c>Host=…;Database=…;Username=…;Ssl Mode=Require</c>), which is exactly what the
/// Azure portal's "Connection strings" (ADO.NET) blade hands operators. So the
/// remainder after <c>postgres:</c> is treated as a native Npgsql string and passed
/// through untouched; a <c>postgres://</c> URL is rejected with an actionable message
/// rather than surfacing Npgsql's opaque parse error.
/// </para>
/// <para>
/// Npgsql has no native keyword to trigger Azure AD auth (unlike SqlClient's
/// <c>Authentication=Active Directory Default</c>), so rstash owns one pseudo-keyword,
/// <c>Auth=Entra</c>, appended to the string. It is the sole token we special-case:
/// detected and stripped here (the .NET analog of Go's <c>extractEntraAuth</c>) so the
/// remaining string is a clean, driver-valid connection string. When set, the caller
/// supplies the password from an Entra access token — the <c>Username</c> (the AAD
/// principal / managed-identity name) must stay in the string.
/// </para>
/// </summary>
public static class PostgresDsn
{
    /// <summary>The connection-string keyword rstash owns to opt into Entra ID auth.</summary>
    private const string AuthKeyword = "Auth";
    private const string EntraValue = "Entra";

    /// <summary>
    /// Splits the connection portion of a <c>postgres:</c> DSN into a native Npgsql
    /// connection string (validated) and whether Entra ID auth was requested.
    /// </summary>
    /// <param name="connection">
    /// The <see cref="DatabaseDsn.ConnectionString"/> of a parsed <c>postgres:</c> DSN —
    /// a native Npgsql keyword string, optionally carrying <c>;Auth=Entra</c>.
    /// </param>
    /// <exception cref="ArgumentException">
    /// The value is empty, is a <c>postgres://</c> URL, carries an unsupported
    /// <c>Auth</c> value, or is not a valid Npgsql connection string.
    /// </exception>
    public static (string ConnectionString, bool UseEntra) Parse(string connection)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(connection);

        if (connection.StartsWith("postgres://", StringComparison.OrdinalIgnoreCase)
            || connection.StartsWith("postgresql://", StringComparison.OrdinalIgnoreCase))
        {
            throw new ArgumentException(
                "postgres:// URL DSNs are not supported (Npgsql accepts only native " +
                "connection strings). Provide the ADO.NET form, e.g. " +
                "postgres:Host=…;Database=…;Username=…;Ssl Mode=Require (append ;Auth=Entra " +
                "for Azure managed-identity auth).",
                nameof(connection));
        }

        var useEntra = false;
        var kept = new List<string>();

        foreach (var segment in connection.Split(';', StringSplitOptions.RemoveEmptyEntries))
        {
            var trimmed = segment.Trim();
            if (trimmed.Length == 0)
            {
                continue;
            }

            var eq = trimmed.IndexOf('=', StringComparison.Ordinal);
            var key = (eq < 0 ? trimmed : trimmed[..eq]).Trim();

            if (!key.Equals(AuthKeyword, StringComparison.OrdinalIgnoreCase))
            {
                kept.Add(trimmed);
                continue;
            }

            var value = eq < 0 ? "" : trimmed[(eq + 1)..].Trim();
            if (!value.Equals(EntraValue, StringComparison.OrdinalIgnoreCase))
            {
                throw new ArgumentException(
                    $"Unsupported Postgres Auth value '{value}'. The only supported value is " +
                    $"'{EntraValue}' (Azure Entra ID managed-identity auth).",
                    nameof(connection));
            }

            useEntra = true;
        }

        // Validate + normalize the remaining native string; throws on unknown keywords.
        var builder = new NpgsqlConnectionStringBuilder(string.Join(';', kept));
        return (builder.ConnectionString, useEntra);
    }
}
