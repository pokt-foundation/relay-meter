package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gojektech/heimdall/httpclient"
	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
	timeUtils "github.com/pokt-foundation/utils-go/time"
	"github.com/stretchr/testify/suite"
)

/* To run the E2E suite use the command `make test_e2e` from the repository root.
The E2E suite also runs on all Pull Requests to the main or staging branches.

The End-to-End test suite uses a Dockerized reproduction of Relay Meter (Collector & API Server)
and all containers it depends on (Relay Meter Postgres DB, PHD & PHD Postgres DB).

The test verifies this data by verifying it can be accessed from the API server's endpoints. */

// Sets up the suite and runs all the tests.
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
			allRelays, err := get[api.TotalRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays",
					id:         "",
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
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
			allAppsRelays, err := get[[]api.AppRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays/apps",
					id:         "",
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
			ts.Equal(test.err, err)
			for _, appRelays := range allAppsRelays {
				ts.Len(appRelays.PublicKey, 37) // Test pub keys have 37 instead of 64 characters
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
			appPubKey types.PortalAppPublicKey
			err       error
		}{
			{
				name:      "Test return value of /relays/apps/{APP_PUB_KEY} endpoint",
				date:      ts.startOfDay,
				appPubKey: testRelay.ApplicationPublicKey,
				err:       nil,
			},
		}

		for _, test := range tests {
			appRelays, err := get[api.AppRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays/apps",
					id:         string(test.appPubKey),
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
			ts.Equal(test.err, err)
			ts.Len(appRelays.PublicKey, 37) // Test pub keys have 37 instead of 64 characters
			ts.Equal(testRelay.ApplicationPublicKey, appRelays.PublicKey)
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
			userID types.UserID
			err    error
		}{
			{
				name:   "Test return value of /relays/users/{USER_ID} endpoint",
				date:   ts.startOfDay,
				userID: testUserID,
				err:    nil,
			},
		}

		for _, test := range tests {
			userRelays, err := get[api.UserRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays/users",
					id:         string(test.userID),
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
			ts.Equal(test.err, err)
			ts.Len(userRelays.User, 6)
			ts.Equal(testUserID, userRelays.User)
			ts.Len(userRelays.PublicKeys, 1)
			ts.Len(userRelays.PublicKeys[0], 37) // Test pub keys have 37 instead of 64 characters
			ts.NotEmpty(userRelays.Count.Success)
			ts.NotEmpty(userRelays.Count.Failure)
			ts.Equal(test.date, userRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), userRelays.To)
		}
	})

	ts.Run("Test_AllPortalAppRelaysEndpoint", func() {
		tests := []struct {
			name           string
			date           time.Time
			emptyRelaysApp types.PortalAppID
			err            error
		}{
			{
				name:           "Test return value of /relays/endpoints endpoint",
				date:           ts.startOfDay,
				emptyRelaysApp: "test_app_3", // test app 3 has no mock relays
				err:            nil,
			},
		}

		for _, test := range tests {
			allEndpointsRelays, err := get[[]api.PortalAppRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays/endpoints",
					id:         "",
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
			ts.Equal(test.err, err)
			for _, endpointRelays := range allEndpointsRelays {
				if endpointRelays.PortalAppID == test.emptyRelaysApp {
					continue
				}

				ts.NotEmpty(endpointRelays.PortalAppID)
				ts.Len(endpointRelays.PublicKeys, 1)
				ts.Len(endpointRelays.PublicKeys[0], 37) // Test pub keys have 37 instead of 64 characters
				ts.NotEmpty(endpointRelays.Count.Success)
				ts.NotEmpty(endpointRelays.Count.Failure)
				ts.Equal(test.date, endpointRelays.From)
				ts.Equal(test.date.AddDate(0, 0, 1), endpointRelays.To)
			}
		}
	})

	ts.Run("Test_PortalAppRelaysEndpoint", func() {
		tests := []struct {
			name        string
			date        time.Time
			portalAppID types.PortalAppID
			err         error
		}{
			{
				name:        "Test return value of /relays/endpoints/{ENDPOINT_ID} endpoint",
				date:        ts.startOfDay,
				portalAppID: "test_app_1",
				err:         nil,
			},
		}

		for _, test := range tests {
			endpointRelays, err := get[api.PortalAppRelaysResponse](
				getOptions{
					baseURL:    relayMeterBaseURL,
					apiKey:     relayMeterAPIKey,
					path:       "v1/relays/endpoints",
					id:         string(test.portalAppID),
					params:     ts.dateParams,
					httpClient: ts.httpClient,
				})
			ts.Equal(test.err, err)
			ts.Len(endpointRelays.PortalAppID, 10)
			ts.Len(endpointRelays.PublicKeys, 1)
			ts.Len(endpointRelays.PublicKeys[0], 37) // Test pub keys have 37 instead of 64 characters
			ts.NotEmpty(endpointRelays.Count.Success)
			ts.NotEmpty(endpointRelays.Count.Failure)
			ts.Equal(test.date, endpointRelays.From)
			ts.Equal(test.date.AddDate(0, 0, 1), endpointRelays.To)
		}
	})
}

/* ---------- Relay Meter Test Suite ---------- */
const (
	relayMeterBaseURL = "http://localhost:9898"
	relayMeterAPIKey  = "test_api_key_1234"
	testUserID        = types.UserID("user_1")
)

var (
	ErrResponseNotOK = errors.New("Response not OK")

	testRelay = TestRelay{
		ApplicationPublicKey: "test_34715cae753e67c75fbb340442e7de8e",
		NodePublicKey:        "test_node_pub_key_02fbcfbad0777942c1da5425bf0105546e7e7f53fffad9",
		Method:               "eth_chainId",
		Blockchain:           "49",
		BlockchainSubdomain:  "fantom-mainnet",
		Origin:               "https://app.test1.io",
		ElapsedTime:          0.16475,
	}
)

type (
	RelayMeterTestSuite struct {
		suite.Suite
		httpClient           *httpclient.Client
		startOfDay, endOfDay time.Time
		dateParams           string
	}
	TestRelay struct {
		ApplicationPublicKey types.PortalAppPublicKey `json:"applicationPublicKey"`
		NodePublicKey        types.PortalAppPublicKey `json:"nodePublicKey"`
		Method               string                   `json:"method"`
		Blockchain           string                   `json:"blockchain"`
		BlockchainSubdomain  string                   `json:"blockchainSubdomain"`
		Origin               types.PortalAppOrigin    `json:"origin"`
		ElapsedTime          float64                  `json:"elapsedTime"`
	}

	getOptions struct {
		baseURL, path, apiKey, id, params string
		httpClient                        *httpclient.Client
	}
)

// SetupSuite runs before each test suite run - takes just over 1 minute to complete
func (ts *RelayMeterTestSuite) SetupSuite() {
	ts.configureTimePeriod() // Configure time period for test

	ts.httpClient = httpclient.NewClient( // HTTP client to test API Server and populate PHD DB
		httpclient.WithHTTPTimeout(10*time.Second), httpclient.WithRetryCount(2),
	)

	<-time.After(1 * time.Second)
}

// Sets the time period vars for the test (00:00.000 to 23:59:59.999 UTC of current day)
func (ts *RelayMeterTestSuite) configureTimePeriod() {
	ts.startOfDay = timeUtils.StartOfDay(time.Now().AddDate(0, 0, -1).UTC())
	ts.endOfDay = ts.startOfDay.AddDate(0, 0, 1).Add(-time.Millisecond)
	ts.dateParams = fmt.Sprintf("?from=%s&to=%s", ts.startOfDay.Format(time.RFC3339), ts.endOfDay.Format(time.RFC3339))
}

// GET test util func
func get[T any](options getOptions) (T, error) {
	rawURL := fmt.Sprintf("%s/%s", options.baseURL, options.path)
	if options.id != "" {
		rawURL = fmt.Sprintf("%s/%s", rawURL, options.id)
	}
	if options.params != "" {
		rawURL = fmt.Sprintf("%s%s", rawURL, options.params)
	}

	headers := http.Header{}
	if options.apiKey != "" {
		headers["Authorization"] = []string{options.apiKey}
	}

	var data T

	fmt.Println("URL HERE", rawURL)

	response, err := options.httpClient.Get(rawURL, headers)
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
