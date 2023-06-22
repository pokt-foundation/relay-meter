package postgresdriver

import (
	"context"
	"testing"
	"time"

	"github.com/pokt-foundation/relay-meter/api"
	"github.com/stretchr/testify/suite"
)

const (
	connectionString = "postgres://postgres:pgpassword@localhost:5432/postgres?sslmode=disable" // pragma: allowlist secret
)

type PGDriverTestSuite struct {
	suite.Suite
	connectionString string
	driver           *PostgresDriver
	today            time.Time
	from             time.Time
	to               time.Time
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

	ts.today = time.Now()

	ts.from = time.Date(2022, time.July, 20, 0, 0, 0, 0, &time.Location{})
	ts.to = ts.from.AddDate(0, 0, 1)

	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_956d67d3ea93cbfe18a",
		Day:         ts.today,
		Success:     5,
		Error:       5,
	}))
	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_6b2faf2e3b061651297",
		Day:         ts.today,
		Success:     5,
		Error:       5,
	}))

	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_956d67d3ea93cbfe18a",
		Day:         ts.from,
		Success:     3,
		Error:       3,
	}))
	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_6b2faf2e3b061651297",
		Day:         ts.from,
		Success:     3,
		Error:       3,
	}))

	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_956d67d3ea93cbfe18a",
		Day:         ts.to,
		Success:     4,
		Error:       4,
	}))
	ts.NoError(ts.driver.WriteHTTPSourceRelayCount(context.Background(), api.HTTPSourceRelayCount{
		PortalAppID: "test_6b2faf2e3b061651297",
		Day:         ts.to,
		Success:     4,
		Error:       4,
	}))
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
