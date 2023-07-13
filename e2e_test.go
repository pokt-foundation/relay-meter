package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gojektech/heimdall/httpclient"
	"github.com/pokt-foundation/relay-meter/api"
	timeUtils "github.com/pokt-foundation/utils-go/time"
	"github.com/stretchr/testify/suite"
)

const testAPIKey = "test_api_key_1234"

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
// TODO: update e2e test to include the new relay-collection logic
func Test_RunSuite_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	testSuite := new(RelayMeterTestSuite)
	suite.Run(t, testSuite)
}

func (ts *RelayMeterTestSuite) Test_RunTests() {
	ts.Run("Test_RelaysEndpoint", func() {
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
			allRelays, err := get[api.TotalRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.NotEmpty(allRelays.Count.Success)
			ts.NotEmpty(allRelays.Count.Failure)
			ts.Equal(test.date, allRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), allRelays.To)
		}
	})

	ts.Run("Test_AllAppRelaysEndpoint", func() {
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
			allAppsRelays, err := get[[]api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/apps", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, appRelays := range allAppsRelays {
				ts.Len(appRelays.Application, 37) // Test pub keys have 37 instead of 64 characters
				ts.NotEmpty(appRelays.Count.Success)
				ts.NotEmpty(appRelays.Count.Failure)
				ts.Equal(test.date, appRelays.From)
				ts.Equal(test.date.AddDate(0, 0, 1), appRelays.To)
			}
		}
	})

	ts.Run("Test_AppRelaysEndpoint", func() {
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
			appRelays, err := get[api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/apps", test.appPubKey, ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Len(appRelays.Application, 37) // Test pub keys have 37 instead of 64 characters
			ts.Equal(ts.TestRelays[0].ApplicationPublicKey, appRelays.Application)
			ts.NotEmpty(appRelays.Count.Success)
			ts.NotEmpty(appRelays.Count.Failure)
			ts.Equal(test.date, appRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), appRelays.To)
		}
	})

	ts.Run("Test_UserRelaysEndpoint", func() {
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
			userRelays, err := get[api.UserRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/users", test.userID, ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Len(userRelays.User, 24)
			ts.Equal(ts.options.testUserID, userRelays.User)
			ts.Len(userRelays.Applications, 1)
			ts.Len(userRelays.Applications[0], 37) // Test pub keys have 37 instead of 64 characters
			ts.NotEmpty(userRelays.Count.Success)
			ts.NotEmpty(userRelays.Count.Failure)
			ts.Equal(test.date, userRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), userRelays.To)
		}
	})

	ts.Run("Test_AllLoadBalancerRelaysEndpoint", func() {
		tests := []struct {
			name           string
			date           time.Time
			emptyRelaysApp string
			err            error
		}{
			{
				name:           "Test return value of /relays/endpoints endpoint",
				date:           ts.startOfDay,
				emptyRelaysApp: "test_cc49729227c22f3934a966d99a6be72b",
				err:            nil,
			},
		}

		for _, test := range tests {
			allEndpointsRelays, err := get[[]api.LoadBalancerRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/endpoints", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, endpointRelays := range allEndpointsRelays {
				if endpointRelays.Applications != nil {
					ts.NotEmpty(endpointRelays.Endpoint)
					ts.Len(endpointRelays.Applications, 1)
					ts.Len(endpointRelays.Applications[0], 37) // Test pub keys have 37 instead of 64 characters
					if endpointRelays.Applications[0] != test.emptyRelaysApp {
						ts.NotEmpty(endpointRelays.Count.Success)
						ts.NotEmpty(endpointRelays.Count.Failure)
					}
					ts.Equal(test.date, endpointRelays.From)
					ts.Equal(test.date.AddDate(0, 0, 1), endpointRelays.To)
				}
			}

			// Must get created endpoint ID  for next test
			ts.testLBID = allEndpointsRelays[0].Endpoint
		}
	})

	ts.Run("Test_LoadBalancerRelaysEndpoint", func() {
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
			endpointRelays, err := get[api.LoadBalancerRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/endpoints", ts.testLBID, ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Len(endpointRelays.Endpoint, 23)
			ts.Len(endpointRelays.Applications, 1)
			ts.Len(endpointRelays.Applications[0], 37) // Test pub keys have 37 instead of 64 characters
			ts.NotEmpty(endpointRelays.Count.Success)
			ts.NotEmpty(endpointRelays.Count.Failure)
			ts.Equal(test.date, endpointRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), endpointRelays.To)
		}
	})

	ts.Run("Test_AllOriginEndpoint", func() {
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
			allOriginRelays, err := get[[]api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v1/relays/origin-classification", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, originRelays := range allOriginRelays {
				ts.Len(originRelays.Origin, 20)
				ts.NotEmpty(originRelays.Count.Success)
				ts.Equal(test.date, originRelays.From)
				ts.Equal(test.date.AddDate(0, 0, 1), originRelays.To)
			}
		}
	})

	ts.Run("Test_OriginEndpoint", func() {
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

			originRelays, err := get[api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v1/relays/origin-classification", url.Host, ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Equal(url.Host, originRelays.Origin)
			ts.Len(originRelays.Origin, 12)
			ts.NotEmpty(originRelays.Count.Success)
			ts.Equal(test.date, originRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), originRelays.To)
		}
	})

	ts.Run("Test_AllAppLatenciesEndpoint", func() {
		tests := []struct {
			name string
			date time.Time
			err  error
		}{
			{
				name: "Test return value of /latency/apps endpoint",
				date: ts.startOfDay,
				err:  nil,
			},
		}

		for _, test := range tests {
			allAppLatencies, err := get[[]api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v1/latency/apps", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, appLatency := range allAppLatencies {
				ts.Len(appLatency.DailyLatency, 24)
				latencyExists := false
				for _, hourlyLatency := range appLatency.DailyLatency {
					ts.NotEmpty(hourlyLatency)
					if hourlyLatency.Latency != 0 {
						ts.NotEmpty(hourlyLatency.Latency)
						latencyExists = true
					}
				}
				ts.True(latencyExists)
				ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
				ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
				ts.Len(appLatency.Application, 37) // Test pub keys have 37 instead of 64 characters
			}
		}
	})

	ts.Run("Test_AppLatenciesEndpoint", func() {
		tests := []struct {
			name      string
			date      time.Time
			appPubKey string
			err       error
		}{
			{
				name:      "Test return value of /latency/apps/{APP_PUB_KEY} endpoint",
				date:      ts.startOfDay,
				appPubKey: ts.TestRelays[0].ApplicationPublicKey,
				err:       nil,
			},
		}

		for _, test := range tests {
			appLatency, err := get[api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v1/latency/apps", test.appPubKey, ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Len(appLatency.DailyLatency, 24)
			latencyExists := false
			for _, hourlyLatency := range appLatency.DailyLatency {
				ts.NotEmpty(hourlyLatency)
				if hourlyLatency.Latency != 0 {
					ts.NotEmpty(hourlyLatency.Latency)
					latencyExists = true
				}
			}
			ts.True(latencyExists)
			ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
			ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
			ts.Len(appLatency.Application, 37) // Test pub keys have 37 instead of 64 characters
		}
	})

}

/* ---------- Relay Meter Test Suite ---------- */
var (
	ctx                      = context.Background()
	ErrResponseNotOK         = errors.New("Response not OK")
	ErrInfluxClientUnhealthy = errors.New("test influx client is unhealthy")
)

type (
	RelayMeterTestSuite struct {
		suite.Suite
		TestRelays           [10]TestRelay
		httpClient           *httpclient.Client
		startOfDay, endOfDay time.Time
		dateParams, testLBID string
		options              TestClientOptions
	}
	TestClientOptions struct {
		phdBaseURL, phdAPIKey, testUserID,
		relayMeterBaseURL string
	}
	TestRelay struct {
		ApplicationPublicKey string  `json:"applicationPublicKey"`
		NodePublicKey        string  `json:"nodePublicKey"`
		Method               string  `json:"method"`
		Blockchain           string  `json:"blockchain"`
		BlockchainSubdomain  string  `json:"blockchainSubdomain"`
		Origin               string  `json:"origin"`
		ElapsedTime          float64 `json:"elapsedTime"`
	}
)

// SetupSuite runs before each test suite run - takes just over 1 minute to complete
func (ts *RelayMeterTestSuite) SetupSuite() {
	ts.configureTimePeriod() // Configure time period for test

	ts.httpClient = httpclient.NewClient( // HTTP client to test API Server and populate PHD DB
		httpclient.WithHTTPTimeout(10*time.Second), httpclient.WithRetryCount(2),
	)

	err := ts.getTestRelays() // Marshals test relay JSON to array of structs
	ts.NoError(err)
	<-time.After(1 * time.Second)

	<-time.After(60 * time.Second) // Wait 65 seconds for collector to run and write to Postgres
}

// Sets the time period vars for the test (00:00.000 to 23:59:59.999 UTC of current day)
func (ts *RelayMeterTestSuite) configureTimePeriod() {
	ts.startOfDay = timeUtils.StartOfDay(time.Now().UTC())
	ts.endOfDay = ts.startOfDay.AddDate(0, 0, 1).Add(-time.Millisecond)
	ts.dateParams = fmt.Sprintf("?from=%s&to=%s", ts.startOfDay.Format(time.RFC3339), ts.endOfDay.Format(time.RFC3339))
}

// Gets the test relays JSON from the testdata directory
func (ts *RelayMeterTestSuite) getTestRelays() error {
	file, err := ioutil.ReadFile("./testdata/mock_relays.json")
	if err != nil {
		return err
	}
	testRelays := [10]TestRelay{}
	err = json.Unmarshal([]byte(file), &testRelays)
	if err != nil {
		return err
	}

	ts.TestRelays = testRelays

	return nil
}

// GET test util func
func get[T any](baseURL, path, id, params, apiKey string, httpClient *httpclient.Client) (T, error) {
	rawURL := fmt.Sprintf("%s/%s", baseURL, path)
	if id != "" {
		rawURL = fmt.Sprintf("%s/%s", rawURL, id)
	}
	if params != "" {
		rawURL = fmt.Sprintf("%s%s", rawURL, params)
	}

	headers := http.Header{}
	if apiKey != "" {
		headers["Authorization"] = []string{apiKey}
	}

	var data T

	response, err := httpClient.Get(rawURL, headers)
	if err != nil {
		return data, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return data, fmt.Errorf("%w. %s", ErrResponseNotOK, http.StatusText(response.StatusCode))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}
