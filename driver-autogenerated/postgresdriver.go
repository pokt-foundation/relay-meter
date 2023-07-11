package postgresdriver

import (
	"database/sql"

	// PQ import is required

	"github.com/jackc/pgx/v5"
	_ "github.com/lib/pq"
)

// The PostgresDriver struct satisfies the Driver interface which defines all database driver methods
type PostgresDriver struct {
	*Queries
	db *sql.DB
	*pgx.Conn
}

/* NewPostgresDriver returns PostgresDriver instance from Postgres connection string */
func NewPostgresDriver(connectionString string) (*PostgresDriver, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}

	driver := &PostgresDriver{
		Queries: New(db),
		db:      db,
	}

	return driver, nil
}

/* NewPostgresDriverFromDBInstance returns PostgresDriver instance from sdl.DB instance */
func NewPostgresDriverFromDBInstance(db *sql.DB) *PostgresDriver {
	driver := &PostgresDriver{
		Queries: New(db),
	}

	return driver
}

/* NewPostgresDriverFromDBInstance returns PostgresDriver instance from sdl.DB instance */
func NewPgxDriverFromDBInstance(conn *pgx.Conn) *PostgresDriver {
	driver := &PostgresDriver{
		Conn:    conn,
		Queries: NewPGX(*conn),
	}

	return driver
}
