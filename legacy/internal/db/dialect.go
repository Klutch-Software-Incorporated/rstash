package db

// Dialect identifies the SQL dialect in use.
type Dialect string

const (
	DialectSQLite    Dialect = "sqlite"
	DialectPostgres  Dialect = "postgres"
	DialectMySQL     Dialect = "mysql"
	DialectSQLServer Dialect = "mssql"
)
