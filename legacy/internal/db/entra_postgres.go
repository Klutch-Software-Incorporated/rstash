package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// entraPostgresScope is the AAD scope for Azure Database for PostgreSQL
// (the "OSS RDBMS" service family — MySQL shares the same audience, but
// rstash only wires Entra auth for Postgres today).
const entraPostgresScope = "https://ossrdbms-aad.database.windows.net/.default"

// extractEntraAuth detects a URL-form Postgres DSN whose query string
// contains auth=entra and returns the DSN with that param stripped plus
// a flag indicating Entra mode. Non-URL DSNs and DSNs without the flag
// pass through unchanged and ok=false.
//
// Example:
//
//	in:  postgres://alice@db.postgres.database.azure.com/rstash?sslmode=require&auth=entra
//	out: postgres://alice@db.postgres.database.azure.com/rstash?sslmode=require, true
func extractEntraAuth(dsn string) (cleaned string, ok bool) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return dsn, false
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn, false
	}
	q := u.Query()
	if !strings.EqualFold(q.Get("auth"), "entra") {
		return dsn, false
	}
	q.Del("auth")
	u.RawQuery = q.Encode()
	return u.String(), true
}

// openEntraPostgres opens a *sql.DB against Azure Database for PostgreSQL
// using Entra ID (AAD) authentication. Each new connection pulled from the
// pool fetches a fresh access token via the azidentity DefaultAzureCredential
// chain (env SP → managed identity → Azure CLI → ...). Tokens expire ~1h;
// because we refresh on every Connect() call and configurePool caps
// ConnMaxLifetime at 5min, pool connections are recycled well before their
// token expires.
//
// The Postgres user in the DSN must match the AAD principal the token is
// issued for (Azure enforces this server-side). For managed identity this
// is the identity's display name or object ID, configured in Azure as an
// AAD admin or member of an AAD-linked role group on the server.
func openEntraPostgres(dsn string) (*sql.DB, error) {
	template, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("entra: create default credential: %w", err)
	}
	return sql.OpenDB(&entraConnector{
		template: template,
		cred:     cred,
		scope:    entraPostgresScope,
	}), nil
}

// entraConnector is a driver.Connector that rewrites the Postgres password
// on every Connect() with a freshly-issued Entra access token, then delegates
// to the pgx stdlib connector.
type entraConnector struct {
	template *pgx.ConnConfig
	cred     azcore.TokenCredential
	scope    string
}

func (c *entraConnector) Connect(ctx context.Context) (driver.Conn, error) {
	tok, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{c.scope},
	})
	if err != nil {
		return nil, fmt.Errorf("entra: fetch access token: %w", err)
	}
	cfg := c.template.Copy()
	cfg.Password = tok.Token
	return stdlib.GetConnector(*cfg).Connect(ctx)
}

func (c *entraConnector) Driver() driver.Driver {
	return stdlib.GetDefaultDriver()
}
