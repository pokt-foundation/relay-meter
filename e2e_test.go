package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
and all containers it depends on (Influx DB, Relay Meter Postgres DB, PHD & PHD Postgres DB).

It follows the flow:
- Populates InfluxDB with 100,000 relays with timestamps across a 24 hour period.
- Influx DB tasks collect and populate time series buckets from the main bucket.
- The Collector will collect these relays from Influx and save them to Postgres.
- The API Server will serve this relay data from Postgres via REST endpoints.

The test verifies this data by verifying it can be accessed from the API server's endpoints. */

// Sets up the suite (~1 minute due to need for Influx tasks and Collector to run) and runs all the tests.
// TODO: update e2e test to include the new relay-collection logic

/* ---------- Relay Meter Test Suite ---------- */

func Test_RunSuite_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	testSuite := new(relayMeterTestSuite)
	suite.Run(t, testSuite)
}

var (
	errResponseNotOK = errors.New("Response not OK")

	today    = timeUtils.StartOfDay(time.Now().UTC())
	tomorrow = today.AddDate(0, 0, 1)
)

type (
	relayMeterTestSuite struct {
		suite.Suite
		httpClient           *httpclient.Client
		dateParams, testLBID string
		options              testClientOptions
	}
	testClientOptions struct {
		phdBaseURL, phdAPIKey,
		relayMeterBaseURL, relayMeterAPIKey string
		testUserID types.UserID
	}
)

// SetupSuite runs before each test suite run - takes just over 1 minute to complete
func (ts *relayMeterTestSuite) SetupSuite() {
	ts.httpClient = httpclient.NewClient( // HTTP client to test API Server and populate PHD DB
		httpclient.WithHTTPTimeout(10*time.Second), httpclient.WithRetryCount(2),
	)

	ts.options = testClientOptions{
		phdBaseURL:        "http://localhost:8080",
		phdAPIKey:         "test_api_key_6789",
		relayMeterBaseURL: "http://localhost:9898",
		relayMeterAPIKey:  "test_api_key_1234",
	}

	ts.dateParams = fmt.Sprintf("?from=%s&to=%s", today.Format(time.RFC3339), today.Format(time.RFC3339))
}

func (ts *relayMeterTestSuite) Test_RunTests() {

	ts.Run("Test_RelaysEndpoint", func() {
		tests := []struct {
			name                string
			expectedTotalRelays api.TotalRelaysResponse
			err                 error
		}{
			{
				name: "Test return value of /relays endpoint",
				expectedTotalRelays: api.TotalRelaysResponse{
					Count: api.RelayCounts{
						Success: 44_902_000,
						Failure: 39_000,
					},
					From: today,
					To:   tomorrow,
				},
				err: nil,
			},
		}

		for _, test := range tests {
			totalRelays, err := get[api.TotalRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays", "", ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Equal(test.expectedTotalRelays, totalRelays)
		}
	})

	ts.Run("Test_AllAppRelaysEndpoint", func() {
		tests := []struct {
			name                 string
			expectedAllAppRelays map[types.PortalAppPublicKey]api.AppRelaysResponse
			err                  error
		}{
			{
				name:                 "Test return value of /relays/apps endpoint",
				expectedAllAppRelays: expectedAllAppRelays,
				err:                  nil,
			},
		}

		for _, test := range tests {
			allAppsRelays, err := get[[]api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/apps", "", ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			ts.Equal(test.expectedAllAppRelays, convertAppRelaysSliceToMap(allAppsRelays))
		}
	})

	ts.Run("Test_AppRelaysEndpoint", func() {
		tests := []struct {
			name              string
			appPubKey         types.PortalAppPublicKey
			expectedAppRelays api.AppRelaysResponse
			err               error
		}{
			{
				name:              "Test return value of /relays/apps/{APP_PUB_KEY} endpoint for app 1",
				appPubKey:         "test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000",
				expectedAppRelays: expectedAllAppRelays["test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000"],
				err:               nil,
			},
			{
				name:              "Test return value of /relays/apps/{APP_PUB_KEY} endpoint for app 2",
				appPubKey:         "test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000",
				expectedAppRelays: expectedAllAppRelays["test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000"],
				err:               nil,
			},
			{
				name:              "Test return value of /relays/apps/{APP_PUB_KEY} endpoint for app 3",
				appPubKey:         "test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000",
				expectedAppRelays: expectedAllAppRelays["test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000"],
				err:               nil,
			},
			{
				name:              "Test return value of /relays/apps/{APP_PUB_KEY} endpoint for app 4",
				appPubKey:         "test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000",
				expectedAppRelays: expectedAllAppRelays["test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000"],
				err:               nil,
			},
		}

		for _, test := range tests {
			appRelays, err := get[api.AppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/apps", string(test.appPubKey), ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			if test.err == nil {
				ts.Equal(test.expectedAppRelays, appRelays)
			}
		}
	})

	ts.Run("Test_UserRelaysEndpoint", func() {
		tests := []struct {
			name               string
			userID             types.UserID
			expectedUserRelays api.UserRelaysResponse
			err                error
		}{
			{
				name:               "Test return value of /relays/users/{USER_ID} endpoint for user_1",
				userID:             "user_1",
				expectedUserRelays: expectedUserRelays["user_1"],
				err:                nil,
			},
			{
				name:               "Test return value of /relays/users/{USER_ID} endpoint for user_3",
				userID:             "user_3",
				expectedUserRelays: expectedUserRelays["user_3"],
				err:                nil,
			},
			{
				name:               "Test return value of /relays/users/{USER_ID} endpoint for user_5",
				userID:             "user_5",
				expectedUserRelays: expectedUserRelays["user_5"],
				err:                nil,
			},
			{
				name:   "Should return an error if user does not exist",
				userID: "user_77",
				err:    errResponseNotOK,
			},
		}

		for _, test := range tests {
			userRelays, err := get[api.UserRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/users", string(test.userID), ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			if test.err == nil {
				userRelays.PublicKeys = sortPublicKeys(userRelays.PublicKeys)
				ts.Equal(test.expectedUserRelays, userRelays)
			}
		}
	})

	ts.Run("Test_AllPortalAppRelaysEndpoint", func() {
		tests := []struct {
			name                       string
			expectedAllPortalAppRelays map[types.PortalAppID]api.PortalAppRelaysResponse
			err                        error
		}{
			{
				name:                       "Test return value of /relays/endpoints endpoint",
				expectedAllPortalAppRelays: expectedAllPortalAppRelays,
				err:                        nil,
			},
		}

		for _, test := range tests {
			allEndpointsRelays, err := get[[]api.PortalAppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/endpoints", "", ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, endpoint := range allEndpointsRelays {
				endpoint.PublicKeys = sortPublicKeys(endpoint.PublicKeys)
			}
			ts.Equal(test.expectedAllPortalAppRelays, convertPortalAppRelaysSliceToMap(allEndpointsRelays))
		}
	})

	ts.Run("Test_PortalAppRelaysEndpoint", func() {
		tests := []struct {
			name                    string
			portalAppID             types.PortalAppID
			expectedPortalAppRelays api.PortalAppRelaysResponse
			err                     error
		}{
			{
				name:                    "Test return value of /relays/endpoints/{ENDPOINT_ID} endpoint",
				portalAppID:             "test_app_1",
				expectedPortalAppRelays: expectedAllPortalAppRelays["test_app_1"],
				err:                     nil,
			},
			{
				name:                    "Test return value of /relays/endpoints/{ENDPOINT_ID} endpoint",
				portalAppID:             "test_app_1",
				expectedPortalAppRelays: expectedAllPortalAppRelays["test_app_1"],
				err:                     nil,
			},
			{
				name:                    "Test return value of /relays/endpoints/{ENDPOINT_ID} endpoint",
				portalAppID:             "test_app_1",
				expectedPortalAppRelays: expectedAllPortalAppRelays["test_app_1"],
				err:                     nil,
			},
			{
				name:        "Should return an error if app does not exist",
				portalAppID: "not_a_key_who_am_i",
				err:         errResponseNotOK,
			},
		}

		for _, test := range tests {
			endpointRelays, err := get[api.PortalAppRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/endpoints", string(test.portalAppID), ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			if test.err == nil {
				ts.Equal(test.expectedPortalAppRelays, endpointRelays)
			}
		}
	})

	/* TODO ---------- Re-enable below tests when Latency and Origin functional again ---------- */

	// ts.Run("Test_AllOriginEndpoint", func() {
	// 	tests := []struct {
	// 		name string
	// 		date time.Time
	// 		err  error
	// 	}{
	// 		{
	// 			name: "Test return value of /relays/origin-classification endpoint",
	// 			date: ts.startOfDay,
	// 			err:  nil,
	// 		},
	// 	}

	// 	for _, test := range tests {
	// 		allOriginRelays, err := get[[]api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v1/relays/origin-classification", "", ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
	// 		ts.Equal(test.err, err)
	// 		for _, originRelays := range allOriginRelays {
	// 			ts.Len(originRelays.Origin, 20)
	// 			ts.NotEmpty(originRelays.Count.Success)
	// 			ts.Equal(test.date, originRelays.From)
	// 			ts.Equal(test.date.AddDate(0, 0, 1), originRelays.To)
	// 		}
	// 	}
	// })

	// ts.Run("Test_OriginEndpoint", func() {
	// 	tests := []struct {
	// 		name   string
	// 		date   time.Time
	// 		origin types.PortalAppOrigin
	// 		err    error
	// 	}{
	// 		{
	// 			name:   "Test return value of /relays/origin-classification/{ORIGIN} endpoint",
	// 			date:   ts.startOfDay,
	// 			origin: ts.TestRelays[0].Origin,
	// 			err:    nil,
	// 		},
	// 	}

	// 	for _, test := range tests {
	// 		// Must parse protocol out of URL eg. https://app.test1.io -> app.test1.io
	// 		url, err := url.Parse(string(test.origin))
	// 		ts.Equal(test.err, err)

	// 		originRelays, err := get[api.OriginClassificationsResponse](ts.options.relayMeterBaseURL, "v1/relays/origin-classification", url.Host, ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
	// 		ts.Equal(test.err, err)
	// 		ts.Equal(types.PortalAppOrigin(url.Host), originRelays.Origin)
	// 		ts.Len(originRelays.Origin, 12)
	// 		ts.NotEmpty(originRelays.Count.Success)
	// 		ts.Equal(test.date, originRelays.From)
	// 		ts.Equal(test.date.AddDate(0, 0, 1), originRelays.To)
	// 	}
	// })

	// ts.Run("Test_AllAppLatenciesEndpoint", func() {
	// 	tests := []struct {
	// 		name string
	// 		date time.Time
	// 		err  error
	// 	}{
	// 		{
	// 			name: "Test return value of /latency/apps endpoint",
	// 			date: ts.startOfDay,
	// 			err:  nil,
	// 		},
	// 	}

	// 	for _, test := range tests {
	// 		allAppLatencies, err := get[[]api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v1/latency/apps", "", ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
	// 		ts.Equal(test.err, err)
	// 		for _, appLatency := range allAppLatencies {
	// 			ts.Len(appLatency.DailyLatency, 24)
	// 			latencyExists := false
	// 			for _, hourlyLatency := range appLatency.DailyLatency {
	// 				ts.NotEmpty(hourlyLatency)
	// 				if hourlyLatency.Latency != 0 {
	// 					ts.NotEmpty(hourlyLatency.Latency)
	// 					latencyExists = true
	// 				}
	// 			}
	// 			ts.True(latencyExists)
	// 			ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
	// 			ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
	// 			ts.Len(appLatency.PublicKey, 37) // Test pub keys have 37 instead of 64 characters
	// 		}
	// 	}
	// })

	// ts.Run("Test_AppLatenciesEndpoint", func() {
	// 	tests := []struct {
	// 		name      string
	// 		date      time.Time
	// 		appPubKey types.PortalAppPublicKey
	// 		err       error
	// 	}{
	// 		{
	// 			name:      "Test return value of /latency/apps/{APP_PUB_KEY} endpoint",
	// 			date:      ts.startOfDay,
	// 			appPubKey: ts.TestRelays[0].ApplicationPublicKey,
	// 			err:       nil,
	// 		},
	// 	}

	// 	for _, test := range tests {
	// 		appLatency, err := get[api.AppLatencyResponse](ts.options.relayMeterBaseURL, "v1/latency/apps", string(test.appPubKey), ts.dateParams, ts.options.relayMeterAPIKey, ts.httpClient)
	// 		ts.Equal(test.err, err)
	// 		ts.Len(appLatency.DailyLatency, 24)
	// 		latencyExists := false
	// 		for _, hourlyLatency := range appLatency.DailyLatency {
	// 			ts.NotEmpty(hourlyLatency)
	// 			if hourlyLatency.Latency != 0 {
	// 				ts.NotEmpty(hourlyLatency.Latency)
	// 				latencyExists = true
	// 			}
	// 		}
	// 		ts.True(latencyExists)
	// 		ts.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
	// 		ts.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
	// 		ts.Len(appLatency.PublicKey, 37) // Test pub keys have 37 instead of 64 characters
	// 	}
	// })

}

/* ---------- Relay Meter Test Data  ---------- */

var (
	expectedAllAppRelays = map[types.PortalAppPublicKey]api.AppRelaysResponse{
		"test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000": {
			Count: api.RelayCounts{
				Success: 3_500_000,
				Failure: 4_000,
			},
			From:      today,
			To:        tomorrow,
			PublicKey: "test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000",
		},
		"test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000": {
			Count: api.RelayCounts{
				Success: 15_700_000,
				Failure: 10_000,
			},
			From:      today,
			To:        tomorrow,
			PublicKey: "test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000",
		},
		"test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000": {
			Count: api.RelayCounts{
				Success: 25_700_000,
				Failure: 24_000,
			},
			From:      today,
			To:        tomorrow,
			PublicKey: "test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000",
		},
		"test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000": {
			Count: api.RelayCounts{
				Success: 2_000,
				Failure: 1_000,
			},
			From:      today,
			To:        tomorrow,
			PublicKey: "test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000",
		},
	}

	expectedUserRelays = map[types.UserID]api.UserRelaysResponse{
		"user_1": {
			User: "user_1",
			Count: api.RelayCounts{
				Success: 3_500_000,
				Failure: 4_000,
			},
			From:       today,
			To:         tomorrow,
			PublicKeys: []types.PortalAppPublicKey{"test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000"},
		},
		"user_3": {
			User: "user_3",
			Count: api.RelayCounts{
				Success: 15_700_000,
				Failure: 10_000,
			},
			From:       today,
			To:         tomorrow,
			PublicKeys: []types.PortalAppPublicKey{"test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000"},
		},
		"user_5": {
			User: "user_5",
			Count: api.RelayCounts{
				Success: 25_702_000,
				Failure: 25_000,
			},
			From: today,
			To:   tomorrow,
			PublicKeys: []types.PortalAppPublicKey{
				"test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000",
				"test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000",
			},
		},
	}

	expectedAllPortalAppRelays = map[types.PortalAppID]api.PortalAppRelaysResponse{
		"test_app_3": {
			Count: api.RelayCounts{
				Success: 25702000,
				Failure: 25000,
			},
			From:        today,
			To:          tomorrow,
			PortalAppID: "test_app_3",
			PublicKeys: []types.PortalAppPublicKey{
				"test_f608500e4fe3e09014fe2411b4a560b5000000000000000000000000000",
				"test_f6a5d8690ecb669865bd752b7796a920000000000000000000000000000",
			},
		},
		"test_app_1": {
			Count: api.RelayCounts{
				Success: 3500000,
				Failure: 4000,
			},
			From:        today,
			To:          tomorrow,
			PortalAppID: "test_app_1",
			PublicKeys: []types.PortalAppPublicKey{
				"test_34715cae753e67c75fbb340442e7de8e000000000000000000000000000",
			},
		},
		"test_app_2": {
			Count: api.RelayCounts{
				Success: 15700000,
				Failure: 10000,
			},
			From:        today,
			To:          tomorrow,
			PortalAppID: "test_app_2",
			PublicKeys: []types.PortalAppPublicKey{
				"test_8237c72345f12d1b1a8b64a1a7f66fa4000000000000000000000000000",
			},
		},
	}
)

func convertAppRelaysSliceToMap(s []api.AppRelaysResponse) map[types.PortalAppPublicKey]api.AppRelaysResponse {
	m := make(map[types.PortalAppPublicKey]api.AppRelaysResponse)
	for _, value := range s {
		m[value.PublicKey] = value
	}
	return m
}

func convertPortalAppRelaysSliceToMap(s []api.PortalAppRelaysResponse) map[types.PortalAppID]api.PortalAppRelaysResponse {
	m := make(map[types.PortalAppID]api.PortalAppRelaysResponse)
	for _, value := range s {
		m[value.PortalAppID] = value
	}
	return m
}

/* ---------- Relay Meter Test Utils Funcs  ---------- */

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
		return data, errResponseNotOK
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

func sortPublicKeys(keys []types.PortalAppPublicKey) []types.PortalAppPublicKey {
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}
