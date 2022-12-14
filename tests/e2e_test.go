//go:build tests

package tests

import (
	"net/url"
	"testing"
	"time"

	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/db"
	"github.com/stretchr/testify/suite"
)

var testSuiteOptions = TestClientOptions{
	InfluxDBOptions: db.InfluxDBOptions{
		URL:                 "http://localhost:8086",
		Token:               "mytoken",
		Org:                 "myorg",
		CurrentBucket:       "mainnetRelayApp10m",
		CurrentOriginBucket: "mainnetOrigin60m",
	},
	mainBucket:        "mainnetRelay",
	main1mBucket:      "mainnetRelayApp1m",
	phdBaseURL:        "http://localhost:8090",
	phdAPIKey:         "test_api_key_6789",
	testUserID:        "12345678fgte0db3b6c63124",
	relayMeterBaseURL: "http://localhost:9898",
}

/* To run the E2E suite use the command `make test_e2e` from the repository root.
The E2E suite also runs on all Pull Requests to the main or staging branches.

The End-to-End test suite uses a Dockerized reproduction of Relay Meter (Collector & API Server)
and all containers it depends on (Influx DB, Relay Meter Postgres DB, PHD & PHD Postgres DB).

It follows the flow:
- Populates InfluxDB with 100,000 relays with timestamps across a 24 hour period.
- Influx DB tasks collect and populate time series buckets from the main bucket.
- The Collector will collect these relays from Influx and save them to Postgres.
- The API Server will serve this relay data from Postgres via REST endpoints.

The test verifies this data by verifying it can be accessed from the API server's endpoints. */

// Sets up the suite (~1 minute due to need for Influx tasks and Collector to run) and runs all the tests.
func Test_RunSuite_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	testSuite := new(RelayMeterTestSuite)
	testSuite.options = testSuiteOptions
	suite.Run(t, testSuite)
}

func (ts *RelayMeterTestSuite) Test_RelaysEndpoint() {
	tests := []struct {
		name string
		date time.Time
		err  error
	}{
		{
			name: "Test return value of /relays endpoint",
			date: ts.startOfDay,
			err:  nil,
		},
	}

	for _, test := range tests {
		allRelays, err := get[api.TotalRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays", "", ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.NotEmpty(allRelays.Count.Success)
		ts.NotEmpty(allRelays.Count.Failure)
		ts.Equal(allRelays.From, test.date)
		ts.Equal(allRelays.To, test.date.AddDate(0, 0, 1))
	}
}

func (ts *RelayMeterTestSuite) Test_AllAppRelaysEndpoint() {
	tests := []struct {
		name string
		date time.Time
		err  error
	}{
		{
			name: "Test return value of /relays/apps endpoint",
			date: ts.startOfDay,
			err:  nil,
		},
	}

	for _, test := range tests {
		allAppsRelays, err := get[[]api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays/apps", "", ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		for _, appRelays := range allAppsRelays {
			ts.Len(appRelays.Application, 64)
			ts.NotEmpty(appRelays.Count.Success)
			ts.NotEmpty(appRelays.Count.Failure)
			ts.Equal(appRelays.From, test.date)
			ts.Equal(appRelays.To, test.date.AddDate(0, 0, 1))
		}
	}
}

func (ts *RelayMeterTestSuite) Test_AppRelaysEndpoint() {
	tests := []struct {
		name      string
		date      time.Time
		appPubKey string
		err       error
	}{
		{
			name:      "Test return value of /relays/apps/{APP_PUB_KEY} endpoint",
			date:      ts.startOfDay,
			appPubKey: ts.TestRelays[0].ApplicationPublicKey,
			err:       nil,
		},
	}

	for _, test := range tests {
		appRelays, err := get[api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays/apps", test.appPubKey, ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.Len(appRelays.Application, 64)
		ts.Equal(ts.TestRelays[0].ApplicationPublicKey, appRelays.Application)
		ts.NotEmpty(appRelays.Count.Success)
		ts.NotEmpty(appRelays.Count.Failure)
		ts.Equal(appRelays.From, test.date)
		ts.Equal(appRelays.To, test.date.AddDate(0, 0, 1))
	}
}

func (ts *RelayMeterTestSuite) Test_UserRelaysEndpoint() {
	tests := []struct {
		name   string
		date   time.Time
		userID string
		err    error
	}{
		{
			name:   "Test return value of /relays/users/{USER_ID} endpoint",
			date:   ts.startOfDay,
			userID: ts.options.testUserID,
			err:    nil,
		},
	}

	for _, test := range tests {
		userRelays, err := get[api.UserRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays/users", test.userID, ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.Len(userRelays.User, 24)
		ts.Equal(ts.options.testUserID, userRelays.User)
		ts.Len(userRelays.Applications, 10)
		ts.Len(userRelays.Applications[0], 64)
		ts.NotEmpty(userRelays.Count.Success)
		ts.NotEmpty(userRelays.Count.Failure)
		ts.Equal(userRelays.From, test.date)
		ts.Equal(userRelays.To, test.date.AddDate(0, 0, 1))
	}
}

func (ts *RelayMeterTestSuite) Test_AllLoadBalancerRelaysEndpoint() {
	tests := []struct {
		name string
		date time.Time
		err  error
	}{
		{
			name: "Test return value of /relays/endpoints endpoint",
			date: ts.startOfDay,
			err:  nil,
		},
	}

	for _, test := range tests {
		allEndpointsRelays, err := get[[]api.LoadBalancerRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays/endpoints", "", ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		for _, endpointRelays := range allEndpointsRelays {
			ts.Len(endpointRelays.Endpoint, 24)
			ts.Len(endpointRelays.Applications, 1)
			ts.Len(endpointRelays.Applications[0], 64)
			ts.NotEmpty(endpointRelays.Count.Success)
			ts.NotEmpty(endpointRelays.Count.Failure)
			ts.Equal(endpointRelays.From, test.date)
			ts.Equal(endpointRelays.To, test.date.AddDate(0, 0, 1))
		}

		// Must get created endpoint ID  for next test
		ts.testLBID = allEndpointsRelays[0].Endpoint
	}
}

func (ts *RelayMeterTestSuite) Test_LoadBalancerRelaysEndpoint() {
	tests := []struct {
		name string
		date time.Time
		err  error
	}{
		{
			name: "Test return value of /relays/endpoints/{ENDPOINT_ID} endpoint",
			date: ts.startOfDay,
			err:  nil,
		},
	}

	for _, test := range tests {
		endpointRelays, err := get[api.LoadBalancerRelaysResponse](ts.options.relayMeterBaseURL, "v0/relays/endpoints", ts.testLBID, ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.Len(endpointRelays.Endpoint, 24)
		ts.Len(endpointRelays.Applications, 1)
		ts.Len(endpointRelays.Applications[0], 64)
		ts.NotEmpty(endpointRelays.Count.Success)
		ts.NotEmpty(endpointRelays.Count.Failure)
		ts.Equal(endpointRelays.From, test.date)
		ts.Equal(endpointRelays.To, test.date.AddDate(0, 0, 1))
	}
}

func (ts *RelayMeterTestSuite) Test_AllOriginEndpoint() {
	tests := []struct {
		name string
		date time.Time
		err  error
	}{
		{
			name: "Test return value of /relays/origin-classification endpoint",
			date: ts.startOfDay,
			err:  nil,
		},
	}

	for _, test := range tests {
		allOriginRelays, err := get[[]api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v0/relays/origin-classification", "", ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		for _, originRelays := range allOriginRelays {
			ts.Len(originRelays.Origin, 20)
			ts.NotEmpty(originRelays.Count.Success)
			ts.Equal(originRelays.From, test.date)
			ts.Equal(originRelays.To, test.date.AddDate(0, 0, 1))
		}
	}
}

func (ts *RelayMeterTestSuite) Test_OriginEndpoint() {
	tests := []struct {
		name   string
		date   time.Time
		origin string
		err    error
	}{
		{
			name:   "Test return value of /relays/origin-classification/{ORIGIN} endpoint",
			date:   ts.startOfDay,
			origin: ts.TestRelays[0].Origin,
			err:    nil,
		},
	}

	for _, test := range tests {
		// Must parse protocol out of URL eg. https://app.test1.io -> app.test1.io
		url, err := url.Parse(test.origin)
		ts.Equal(test.err, err)

		originRelays, err := get[api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v0/relays/origin-classification", url.Host, ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.Equal(url.Host, originRelays.Origin)
		ts.Len(originRelays.Origin, 12)
		ts.NotEmpty(originRelays.Count.Success)
		ts.Equal(originRelays.From, test.date)
		ts.Equal(originRelays.To, test.date.AddDate(0, 0, 1))
	}
}

func (ts *RelayMeterTestSuite) Test_AllAppLatenciesEndpoint() {
	tests := []struct {
		name                  string
		date                  time.Time
		expectedHourlyLatency map[string]float64
		err                   error
	}{
		{
			name: "Test return value of /latency/apps endpoint",
			date: ts.startOfDay,
			expectedHourlyLatency: map[string]float64{
				ts.TestRelays[0].ApplicationPublicKey: 0.16475,
				ts.TestRelays[1].ApplicationPublicKey: 0.20045,
				ts.TestRelays[2].ApplicationPublicKey: 0.08137,
				ts.TestRelays[3].ApplicationPublicKey: 0.15785,
				ts.TestRelays[4].ApplicationPublicKey: 0.05467,
				ts.TestRelays[5].ApplicationPublicKey: 0.1093,
				ts.TestRelays[6].ApplicationPublicKey: 0.2205,
				ts.TestRelays[7].ApplicationPublicKey: 0.0932,
				ts.TestRelays[8].ApplicationPublicKey: 0.1162,
				ts.TestRelays[9].ApplicationPublicKey: 0.0814,
			},
			err: nil,
		},
	}

	for _, test := range tests {
		allAppLatencies, err := get[[]api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v0/latency/apps", "", ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		for _, appLatency := range allAppLatencies {
			ts.Len(appLatency.DailyLatency, 24)
			for _, hourlyLatency := range appLatency.DailyLatency {
				ts.NotEmpty(hourlyLatency)
				if hourlyLatency.Latency != 0 {
					ts.Equal(test.expectedHourlyLatency[appLatency.Application], hourlyLatency.Latency)
				}
			}
			ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
			ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
			ts.Len(appLatency.Application, 64)
		}
	}
}

func (ts *RelayMeterTestSuite) Test_AppLatenciesEndpoint() {
	tests := []struct {
		name                  string
		date                  time.Time
		appPubKey             string
		expectedHourlyLatency map[string]float64
		err                   error
	}{
		{
			name:      "Test return value of /latency/apps/{APP_PUB_KEY} endpoint",
			date:      ts.startOfDay,
			appPubKey: ts.TestRelays[0].ApplicationPublicKey,
			expectedHourlyLatency: map[string]float64{
				ts.TestRelays[0].ApplicationPublicKey: 0.16475,
				ts.TestRelays[1].ApplicationPublicKey: 0.20045,
				ts.TestRelays[2].ApplicationPublicKey: 0.08137,
				ts.TestRelays[3].ApplicationPublicKey: 0.15785,
				ts.TestRelays[4].ApplicationPublicKey: 0.05467,
				ts.TestRelays[5].ApplicationPublicKey: 0.1093,
				ts.TestRelays[6].ApplicationPublicKey: 0.2205,
				ts.TestRelays[7].ApplicationPublicKey: 0.0932,
				ts.TestRelays[8].ApplicationPublicKey: 0.1162,
				ts.TestRelays[9].ApplicationPublicKey: 0.0814,
			},
			err: nil,
		},
	}

	for _, test := range tests {
		appLatency, err := get[api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v0/latency/apps", test.appPubKey, ts.dateParams, "", ts.httpClient)
		ts.Equal(test.err, err)
		ts.Len(appLatency.DailyLatency, 24)
		for _, hourlyLatency := range appLatency.DailyLatency {
			ts.NotEmpty(hourlyLatency)
			if hourlyLatency.Latency != 0 {
				ts.Equal(test.expectedHourlyLatency[appLatency.Application], hourlyLatency.Latency)
			}
		}
		ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
		ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
		ts.Len(appLatency.Application, 64)
	}
}
