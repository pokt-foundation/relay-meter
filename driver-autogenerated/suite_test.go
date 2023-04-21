package postgresdriver

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

const (
	connectionString = "postgres://postgres:pgpassword@localhost:5432/postgres?sslmode=disable" // pragma: allowlist secret
)

type PGDriverTestSuite struct {
	suite.Suite
	connectionString string
	driver           *PostgresDriver
}

func Test_RunPGDriverSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping driver test")
	}

	testSuite := new(PGDriverTestSuite)
	testSuite.connectionString = connectionString

	suite.Run(t, testSuite)
}

// SetupSuite runs before each test suite run
func (ts *PGDriverTestSuite) SetupSuite() {
	ts.NoError(ts.initPostgresDriver())
}

// Initializes a real instance of the Postgres driver that connects to the test Postgres Docker container
func (ts *PGDriverTestSuite) initPostgresDriver() error {
	driver, err := NewPostgresDriver(ts.connectionString)
	if err != nil {
		return err
	}
	ts.driver = driver

	return nil
}
