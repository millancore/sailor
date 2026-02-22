package docker

import (
	"fmt"
	"path/filepath"

	"github.com/millancore/sailor/internal/env"
)

// DBType represents a supported database backend.
type DBType string

const (
	DBTypeMySQL    DBType = "mysql"
	DBTypePostgres DBType = "pgsql"
)

// DBInfo holds connection details for the detected database.
type DBInfo struct {
	Container string
	Type      DBType
	User      string
	Password  string
}

// DetectDB reads DB_CONNECTION from mainDir/.env, finds the running container,
// and returns connection details.
func DetectDB(mainDir string) (DBInfo, error) {
	mainEnvPath := filepath.Join(mainDir, ".env")
	conn := env.Get(mainEnvPath, "DB_CONNECTION", "mysql")

	switch conn {
	case "pgsql", "postgres", "postgresql":
		container, err := FindPostgresContainer(mainDir)
		if err != nil {
			return DBInfo{}, err
		}
		user := env.Get(mainEnvPath, "DB_USERNAME", "postgres")
		password := env.Get(mainEnvPath, "DB_PASSWORD", "password")
		return DBInfo{
			Container: container,
			Type:      DBTypePostgres,
			User:      user,
			Password:  password,
		}, nil

	default: // mysql, mariadb, or unset
		container, err := FindMySQLContainer(mainDir)
		if err != nil {
			return DBInfo{}, err
		}
		password := env.Get(mainEnvPath, "DB_PASSWORD", "password")
		return DBInfo{
			Container: container,
			Type:      DBTypeMySQL,
			User:      "root",
			Password:  password,
		}, nil
	}
}

// DBIsReachable returns true if the database is reachable.
func DBIsReachable(info DBInfo) bool {
	switch info.Type {
	case DBTypePostgres:
		return PostgresIsReachable(info.Container, info.User)
	default:
		return MySQLIsReachable(info.Container)
	}
}

// DBCreateDB creates a database.
func DBCreateDB(info DBInfo, dbName string) error {
	switch info.Type {
	case DBTypePostgres:
		return PostgresCreateDB(info.Container, info.User, info.Password, dbName)
	default:
		return MySQLCreateDB(info.Container, info.Password, dbName)
	}
}

// DBDropDB drops a database.
func DBDropDB(info DBInfo, dbName string) error {
	switch info.Type {
	case DBTypePostgres:
		return PostgresDropDB(info.Container, info.User, info.Password, dbName)
	default:
		return MySQLDropDB(info.Container, info.Password, dbName)
	}
}

// DBHasTables checks if a database has any tables.
func DBHasTables(info DBInfo, dbName string) bool {
	switch info.Type {
	case DBTypePostgres:
		return PostgresHasTables(info.Container, info.User, info.Password, dbName)
	default:
		return MySQLHasTables(info.Container, info.Password, dbName)
	}
}

// DBDump dumps a database.
func DBDump(info DBInfo, dbName string, schemaOnly bool) (string, error) {
	switch info.Type {
	case DBTypePostgres:
		return PostgresDump(info.Container, info.User, info.Password, dbName, schemaOnly)
	default:
		return MySQLDump(info.Container, info.Password, dbName, schemaOnly)
	}
}

// DBImport imports SQL into a database.
func DBImport(info DBInfo, dbName, sql string) error {
	switch info.Type {
	case DBTypePostgres:
		return PostgresImport(info.Container, info.User, info.Password, dbName, sql)
	default:
		return MySQLImport(info.Container, info.Password, dbName, sql)
	}
}

// DBTypeName returns a human-readable name for the DB type.
func DBTypeName(t DBType) string {
	switch t {
	case DBTypePostgres:
		return "PostgreSQL"
	default:
		return "MySQL"
	}
}

// DBNotReachableError returns an error message for unreachable DB.
func DBNotReachableError(t DBType) string {
	return fmt.Sprintf("%s not reachable — is the main branch running? (sail up -d)", DBTypeName(t))
}
