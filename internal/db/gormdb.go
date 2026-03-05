package db

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenGORM opens a GORM database connection for the given DSN.
// Supported schemes: sqlite:, postgres:, mysql:, mssql:.
// For SQLite, the dsn may be a bare path or "sqlite:path".
func OpenGORM(dsn string) (*gorm.DB, Dialect, error) {
	scheme, connStr := parseDSN(dsn)

	gormCfg := &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	var dialector gorm.Dialector
	var dialect Dialect

	switch scheme {
	case "sqlite":
		dialect = DialectSQLite
		dialector = sqlite.Open(connStr)

	case "postgres":
		dialect = DialectPostgres
		dialector = postgres.Open(connStr)

	case "mysql":
		dialect = DialectMySQL
		dialector = mysql.Open(connStr)

	case "mssql":
		dialect = DialectSQLServer
		dialector = sqlserver.Open(connStr)

	default:
		return nil, "", fmt.Errorf("unsupported database scheme: %q (use sqlite:, postgres:, mysql:, or mssql:)", scheme)
	}

	gormDB, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		// If the database doesn't exist, try to create it automatically.
		if dialect != DialectSQLite {
			if created := tryCreateDatabase(dialect, connStr); created {
				// Retry the connection.
				gormDB, err = gorm.Open(dialector, gormCfg)
			}
		}
		if err != nil {
			return nil, "", fmt.Errorf("open %s database: %w", scheme, err)
		}
	}

	// Per-dialect initialization.
	if err := initDialect(gormDB, dialect); err != nil {
		return nil, "", err
	}

	// Configure connection pool.
	if err := configurePool(gormDB, dialect); err != nil {
		return nil, "", err
	}

	return gormDB, dialect, nil
}

// parseDSN extracts the scheme and connection string from a DSN.
// "sqlite:path" → ("sqlite", "path"), "postgres:connstr" → ("postgres", "connstr"), etc.
// A bare path without a scheme is treated as SQLite.
func parseDSN(dsn string) (scheme, connStr string) {
	// Handle known multi-char schemes first.
	for _, prefix := range []string{"sqlite:", "postgres:", "mysql:", "mssql:"} {
		if strings.HasPrefix(dsn, prefix) {
			return strings.TrimSuffix(prefix, ":"), dsn[len(prefix):]
		}
	}
	// No recognized scheme — treat as a bare SQLite path.
	return "sqlite", dsn
}

// initDialect applies per-dialect initialization after opening a GORM connection.
func initDialect(gormDB *gorm.DB, dialect Dialect) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}

	switch dialect {
	case DialectSQLite:
		pragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys=ON",
			"PRAGMA case_sensitive_like=ON",
		}
		for _, p := range pragmas {
			if _, err := sqlDB.Exec(p); err != nil {
				slog.Warn("SQLite pragma failed", "pragma", p, "error", err)
			}
		}

	case DialectMySQL:
		// Ensure binary collation for case-sensitive LIKE on path columns.
		// This is handled at the column/table level via GORM tags.

	case DialectPostgres:
		// PostgreSQL LIKE is case-sensitive by default. No special init needed.

	case DialectSQLServer:
		// SQL Server LIKE is case-sensitive if the collation is binary.
		// We rely on the database collation being set appropriately.
	}

	return nil
}

// configurePool sets connection pool parameters on the underlying *sql.DB.
// SQLite is single-writer so it gets 1 connection; network databases get a
// larger pool suitable for concurrent access.
func configurePool(gormDB *gorm.DB, dialect Dialect) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB for pool config: %w", err)
	}

	if dialect == DialectSQLite {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	}
	return nil
}

// tryCreateDatabase connects to the server's default database and attempts to
// CREATE DATABASE for the target database. Returns true if the database was
// created successfully.
func tryCreateDatabase(dialect Dialect, connStr string) bool {
	dbName, bootstrapDSN := bootstrapConnStr(dialect, connStr)
	if dbName == "" || bootstrapDSN == "" {
		return false
	}

	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var dialector gorm.Dialector
	switch dialect {
	case DialectPostgres:
		dialector = postgres.Open(bootstrapDSN)
	case DialectMySQL:
		dialector = mysql.Open(bootstrapDSN)
	case DialectSQLServer:
		dialector = sqlserver.Open(bootstrapDSN)
	default:
		return false
	}

	tmpDB, err := gorm.Open(dialector, cfg)
	if err != nil {
		return false
	}
	sqlDB, err := tmpDB.DB()
	if err != nil {
		return false
	}
	defer sqlDB.Close()

	// Database names can't be parameterized; validate it's a simple identifier.
	if !isSimpleIdentifier(dbName) {
		return false
	}

	if err := tmpDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		slog.Debug("auto-create database failed", "database", dbName, "error", err)
		return false
	}

	slog.Info("auto-created database", "database", dbName)
	return true
}

// isSimpleIdentifier returns true if s contains only alphanumeric chars and underscores.
var simpleIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isSimpleIdentifier(s string) bool {
	return simpleIdentRe.MatchString(s)
}

// bootstrapConnStr returns the database name and a modified connection string
// that connects to the server's default/admin database instead of the target.
func bootstrapConnStr(dialect Dialect, connStr string) (dbName, bootstrap string) {
	switch dialect {
	case DialectPostgres:
		// Postgres key=value format: "host=localhost user=x dbname=mydb ..."
		// Replace dbname=X with dbname=postgres.
		parts := strings.Fields(connStr)
		var name string
		var out []string
		for _, p := range parts {
			if strings.HasPrefix(p, "dbname=") {
				name = strings.TrimPrefix(p, "dbname=")
				out = append(out, "dbname=postgres")
			} else {
				out = append(out, p)
			}
		}
		if name == "" {
			return "", ""
		}
		return name, strings.Join(out, " ")

	case DialectMySQL:
		// MySQL DSN: "user:pass@tcp(host:port)/dbname?params"
		slashIdx := strings.LastIndex(connStr, "/")
		if slashIdx < 0 {
			return "", ""
		}
		dbAndParams := connStr[slashIdx+1:]
		prefix := connStr[:slashIdx+1]
		name := dbAndParams
		params := ""
		if qIdx := strings.Index(dbAndParams, "?"); qIdx >= 0 {
			name = dbAndParams[:qIdx]
			params = dbAndParams[qIdx:]
		}
		if name == "" {
			return "", ""
		}
		return name, prefix + "mysql" + params

	case DialectSQLServer:
		// MSSQL URL: "sqlserver://sa:pass@host:port?database=mydb&other=x"
		u, err := url.Parse("sqlserver://" + strings.TrimPrefix(connStr, "sqlserver://"))
		if err != nil {
			return "", ""
		}
		q := u.Query()
		name := q.Get("database")
		if name == "" {
			return "", ""
		}
		q.Set("database", "master")
		u.RawQuery = q.Encode()
		return name, u.String()
	}
	return "", ""
}
