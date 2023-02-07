package tests

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
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/db"
	timeUtils "github.com/pokt-foundation/utils-go/time"
	"github.com/stretchr/testify/suite"
)

var testSuiteOptions = TestClientOptions{
	InfluxDBOptions: db.InfluxDBOptions{
		URL:                 "http://localhost:8086",
		Token:               "mytoken",
		Org:                 "myorg",
		CurrentBucket:       "mainnetRelayApp10m",
		DailyBucket:         "mainnetRelayApp1d",
		CurrentOriginBucket: "mainnetOrigin60m",
	},
	mainBucket:        "mainnetRelay",
	main1mBucket:      "mainnetRelayApp1m",
	phdBaseURL:        "http://localhost:8090",
	phdAPIKey:         "test_api_key_6789",
	testUserID:        "test_user_1dbffbdfeeb225",
	relayMeterBaseURL: "http://localhost:9898",
}

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
func Test_RunSuite_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	testSuite := new(RelayMeterTestSuite)
	testSuite.options = testSuiteOptions
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
			allEndpointsRelays, err := get[[]api.LoadBalancerRelaysResponse](ts.options.relayMeterBaseURL, "v1/relays/endpoints", "", ts.dateParams, testAPIKey, ts.httpClient)
			ts.Equal(test.err, err)
			for _, endpointRelays := range allEndpointsRelays {
				if endpointRelays.Applications != nil {
					ts.NotEmpty(endpointRelays.Endpoint)
					ts.Len(endpointRelays.Applications, 1)
					ts.Len(endpointRelays.Applications[0], 37) // Test pub keys have 37 instead of 64 characters
					ts.NotEmpty(endpointRelays.Count.Success)
					ts.NotEmpty(endpointRelays.Count.Failure)
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
		TestRelays                  [10]TestRelay
		influxClient                influxdb2.Client
		httpClient                  *httpclient.Client
		startOfDay, endOfDay        time.Time
		dateParams, orgID, testLBID string
		options                     TestClientOptions
	}
	TestClientOptions struct {
		db.InfluxDBOptions
		mainBucket, main1mBucket, orgID,
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
	<-time.After(20 * time.Second) // Wait for Docker env to finish setting up

	ts.configureTimePeriod() // Configure time period for test

	ts.httpClient = httpclient.NewClient( // HTTP client to test API Server and populate PHD DB
		httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0),
	)

	err := ts.getTestRelays() // Marshals test relay JSON to array of structs
	ts.NoError(err)

	err = ts.initInflux() // Setup Influx client to interact with Influx DB
	ts.NoError(err)

	err = ts.resetInfluxBuckets() // Ensure Influx buckets are empty at start of test
	ts.NoError(err)
	<-time.After(5 * time.Second) // Wait for bucket creation to complete

	err = ts.populateInfluxRelays() // Populate Influx DB with 100,000 relays
	ts.NoError(err)
	<-time.After(10 * time.Second) // Wait for relay population to complete

	err = ts.runInfluxTasks() // Manually run the Influx tasks (takes ~40 seconds)
	ts.NoError(err)
	<-time.After(30 * time.Second) // Wait 30 seconds for collector to run and write to Postgres
}

// Sets the time period vars for the test (00:00.000 to 23:59:59.999 UTC of current day)
func (ts *RelayMeterTestSuite) configureTimePeriod() {
	ts.startOfDay = timeUtils.StartOfDay(time.Now().UTC())
	ts.endOfDay = ts.startOfDay.AddDate(0, 0, 1).Add(-time.Millisecond)
	ts.dateParams = fmt.Sprintf("?from=%s&to=%s", ts.startOfDay.Format(time.RFC3339), ts.endOfDay.Format(time.RFC3339))
}

// Initializes the Influx client and returns it alongside the org ID
func (ts *RelayMeterTestSuite) initInflux() error {
	influxClient := influxdb2.NewClientWithOptions(
		ts.options.URL, ts.options.Token,
		influxdb2.DefaultOptions().SetBatchSize(4_000),
	)
	health, err := influxClient.Health(ctx)
	if err != nil {
		return err
	}
	if *health.Message != "ready for queries and writes" {
		return ErrInfluxClientUnhealthy
	}
	ts.influxClient = influxClient

	dOrg, err := influxClient.OrganizationsAPI().FindOrganizationByName(ctx, ts.options.Org)
	if err != nil {
		return err
	}
	ts.orgID = *dOrg.Id

	return nil
}

// Resets the Influx bucket each time the collector test runs
func (ts *RelayMeterTestSuite) resetInfluxBuckets() error {
	bucketsAPI := ts.influxClient.BucketsAPI()
	usedBuckets := []string{
		ts.options.mainBucket,
		ts.options.main1mBucket,
		ts.options.DailyBucket,
		ts.options.CurrentBucket,
		ts.options.CurrentOriginBucket,
	}

	for _, bucketName := range usedBuckets {
		bucket, err := bucketsAPI.FindBucketByName(ctx, bucketName)
		if err == nil {
			err = ts.influxClient.BucketsAPI().DeleteBucketWithID(ctx, *bucket.Id)
			if err != nil {
				return err
			}
		}
		_, err = ts.influxClient.BucketsAPI().CreateBucketWithNameWithID(ctx, ts.orgID, bucketName)
		if err != nil {
			return err
		}

		<-time.After(5 * time.Second)
	}

	<-time.After(5 * time.Second) // Wait for bucket creation to complete

	return nil
}

// Initializes the Influx tasks used to populate each bucket from the main bucket
func (ts *RelayMeterTestSuite) runInfluxTasks() error {
	tasksAPI := ts.influxClient.TasksAPI()

	startOfDayFormatted, endOfDayFormatted := ts.startOfDay.Format(time.RFC3339), ts.endOfDay.Format(time.RFC3339)
	tasksToInit := map[string]string{
		"app-1m":            fmt.Sprintf(app1mStringRaw, ts.options.mainBucket, startOfDayFormatted, endOfDayFormatted, ts.options.main1mBucket),
		"app-10m":           fmt.Sprintf(app10mStringRaw, ts.options.main1mBucket, startOfDayFormatted, endOfDayFormatted, ts.options.CurrentBucket),
		"app-1d":            fmt.Sprintf(app1dStringRaw, ts.options.CurrentBucket, startOfDayFormatted, endOfDayFormatted, ts.options.DailyBucket),
		"origin-sample-60m": fmt.Sprintf(origin60mStringRaw, ts.options.mainBucket, startOfDayFormatted, endOfDayFormatted, ts.options.CurrentOriginBucket),
	}

	existingTasks, err := tasksAPI.FindTasks(ctx, nil)
	if err != nil {
		return err
	}

	for taskName, fluxTask := range tasksToInit {
		for _, existingTask := range existingTasks {
			if existingTask.Name == taskName {
				err = tasksAPI.DeleteTaskWithID(ctx, existingTask.Id)
				if err != nil {
					return err
				}
			}
		}

		task, err := tasksAPI.CreateTaskWithEvery(ctx, taskName, fluxTask, "1h", ts.orgID)
		if err != nil {
			return err
		}

		_, err = tasksAPI.RunManually(ctx, task)
		if err != nil {
			return err
		}

		<-time.After(20 * time.Second) // Wait for task to complete
	}

	return nil
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

// Saves a test batch of relays to the InfluxDB mainBucket
func (ts *RelayMeterTestSuite) populateInfluxRelays() error {
	numberOfRelays := 100_000

	timestampInterval := (24 * time.Hour) / time.Duration(numberOfRelays)

	writeAPI := ts.influxClient.WriteAPI(ts.options.Org, ts.options.mainBucket)

	for i := 0; i < numberOfRelays; i++ {
		index := i % 10
		relay := ts.TestRelays[index]

		relayTimestamp := ts.startOfDay.Add(timestampInterval * time.Duration(i+1))
		if i == 0 {
			relayTimestamp.Add(time.Millisecond * 10)
		}
		if i == numberOfRelays-1 {
			relayTimestamp.Add(-time.Millisecond * 10)
		}

		var result string
		if i%9 != 0 {
			result = "200"
		} else {
			result = "500"
		}

		relayPoint := influxdb2.NewPoint(
			"relay",
			map[string]string{
				"applicationPublicKey": relay.ApplicationPublicKey,
				"nodePublicKey":        relay.NodePublicKey,
				"method":               relay.Method,
				"result":               result,
				"blockchain":           relay.Blockchain,
				"blockchainSubdomain":  relay.BlockchainSubdomain,
				"region":               "us-east-2",
			},
			map[string]interface{}{
				"bytes":       12345,
				"elapsedTime": relay.ElapsedTime,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(relayPoint)

		originPoint := influxdb2.NewPoint(
			"origin",
			map[string]string{
				"applicationPublicKey": relay.ApplicationPublicKey,
			},
			map[string]interface{}{
				"origin": relay.Origin,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(originPoint)
	}

	writeAPI.Flush()

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

// These Flux & JSON strings must have values filled in programmatically so must be saved here in the Go file
const (
	// Influx Tasks Flux
	app1mStringRaw = `from(bucket: "%s")
	|> range(start: %s, stop: %s)
	|> filter(fn: (r) => r._field == "elapsedTime" and (r.method != "chaincheck" and r.method != "synccheck"))
	|> drop(columns: ["host", "method"])
	|> map(
		fn: (r) => ({
			_time: r._time,
			_start: r._start,
			_stop: r._stop,
			_measurement: r._measurement,
			_field: r._field,
			_value: r._value,
			region: r.region,
			blockchain: r.blockchain,
			result: r.result,
			applicationPublicKey: r.applicationPublicKey,
			nodePublicKey: if r.nodePublicKey =~ /^fallback/ then "fallback" else "network",
		}),
	)
	|> window(every: 1ms)
	|> reduce(
		fn: (r, accumulator) => ({count: accumulator.count + 1, total: accumulator.total + r._value, elapsedTime: (accumulator.total + r._value) / float(v: accumulator.count)}),
		identity: {count: 1, total: 0.0, elapsedTime: 0.0},
	)
	|> to(
		bucket: "%s",
		org: "myorg",
		timeColumn: "_stop",
		fieldFn: (r) => ({"count": r.count - 1, "elapsedTime": r.elapsedTime}),
	)`
	app10mStringRaw = `from(bucket: "%s")
	|> range(start: %s, stop: %s)
	|> filter(fn: (r) => r._field == "count" or r._field == "elapsedTime")
	|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
	|> window(every: 1ms)
	|> reduce(
		fn: (r, accumulator) => ({count: r.count + accumulator.count, total: accumulator.total + float(v: r.count) * r.elapsedTime, elapsedTime: (accumulator.total + r.elapsedTime) / float(v: accumulator.count)}),
		identity: {count: 1, total: 0.0, elapsedTime: 0.0},
	)
	|> to(
		bucket: "%s",
		org: "myorg",
		timeColumn: "_stop",
		fieldFn: (r) => ({"count": r.count - 1, "elapsedTime": r.elapsedTime}),
	)`
	app1dStringRaw = `from(bucket: "%s")
	|> range(start: %s, stop: %s)
	|> filter(fn: (r) => r._field == "count" or r._field == "elapsedTime")
	|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
	|> window(every: 1ms)
	|> reduce(
		fn: (r, accumulator) => ({count: r.count + accumulator.count, total: accumulator.total + float(v: r.count) * r.elapsedTime, elapsedTime: (accumulator.total + r.elapsedTime) / float(v: accumulator.count)}),
		identity: {count: 1, total: 0.0, elapsedTime: 0.0},
	)
	|> to(
		bucket: "%s",
		org: "myorg",
		timeColumn: "_stop",
		fieldFn: (r) => ({"count": r.count - 1, "elapsedTime": r.elapsedTime}),
	)`
	origin60mStringRaw = `from(bucket: "%s")
	|> range(start: %s, stop: %s)
	|> filter(fn: (r) => (r["_measurement"] == "origin"))
	|> filter(fn: (r) => (r["_field"] == "origin"))
	|> window(every: 1ms)
	|> reduce(
        fn: (r, accumulator) => ({count: accumulator.count + 1, origin: r._value}),
        identity: {origin: "", count: 1},
    )
	|> to(
		bucket: "%s",
		org: "myorg",
		timeColumn: "_stop",
		tagColumns: ["origin", "applicationPublicKey"],
		fieldFn: (r) => ({"count": r.count - 1}),
	)`
)
