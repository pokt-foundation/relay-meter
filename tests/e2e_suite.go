//go:build tests

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gojektech/heimdall/httpclient"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxDomain "github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/pokt-foundation/portal-api-go/repository"
	"github.com/pokt-foundation/relay-meter/db"
	stringUtils "github.com/pokt-foundation/utils-go/strings"
	"github.com/stretchr/testify/suite"
)

var (
	ctx                    = context.Background()
	ErrResponseNotOK error = errors.New("Response not OK")
)

// TODO move this to utils-go
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

type (
	RelayMeterTestSuite struct {
		suite.Suite
		Source               db.Source
		TestRelays           []TestRelay
		influxClient         influxdb2.Client
		httpClient           *httpclient.Client
		tasks                []*influxDomain.Task
		startOfDay, endOfDay time.Time
		dateParams           string
		orgID                string
		testEndpointID       string
		options              TestClientOptions
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

func (ts *RelayMeterTestSuite) SetupSuite() {
	startOfDay := startOfDay(time.Now().UTC())
	endOfDay := startOfDay.AddDate(0, 0, 1).Add(-time.Millisecond)
	ts.startOfDay = startOfDay
	ts.endOfDay = endOfDay
	ts.dateParams = fmt.Sprintf("?from=%s&to=%s", startOfDay.Format(time.RFC3339), endOfDay.Format(time.RFC3339))

	err := ts.initInflux()
	ts.NoError(err)

	ts.resetInfluxBuckets()
	err = ts.initInfluxTasks()
	ts.NoError(err)

	err = ts.getTestRelays()
	ts.NoError(err)

	ts.Source = db.NewInfluxDBSource(ts.options.InfluxDBOptions)
	ts.httpClient = httpclient.NewClient(httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0))

	/* Populate the databases */
	ts.populatePocketHTTPDB()
	ts.populateInfluxRelays()
	/* Manually run the Influx tasks to populate time scale buckets from main bucket */
	ts.runInfluxTasks()

	time.Sleep(30 * time.Second) // Wait for collector to run and write to Postgres
}

// Initializes the Influx client and returns it alongside the org ID
func (ts *RelayMeterTestSuite) initInflux() error {
	influxClient := influxdb2.NewClientWithOptions(
		ts.options.URL, ts.options.Token,
		influxdb2.DefaultOptions().SetBatchSize(4_000),
	)
	health, err := influxClient.Health(context.Background())
	if err != nil {
		return err
	}
	if *health.Message != "ready for queries and writes" {
		return errors.New("test influx client is unhealthy")
	}
	ts.influxClient = influxClient

	dOrg, err := influxClient.OrganizationsAPI().FindOrganizationByName(context.Background(), ts.options.Org)
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
	}

	return nil
}

// Initializes the Influx tasks used to populate each bucket from the main bucket
func (ts *RelayMeterTestSuite) initInfluxTasks() error {
	tasksAPI := ts.influxClient.TasksAPI()

	startOfDayFormatted, endOfDayFormatted := ts.startOfDay.Format(time.RFC3339), ts.endOfDay.Format(time.RFC3339)
	tasksToInit := map[string]string{
		"app-1m":            fmt.Sprintf(app1mStringRaw, ts.options.mainBucket, startOfDayFormatted, endOfDayFormatted, ts.options.main1mBucket),
		"app-10m":           fmt.Sprintf(app10mStringRaw, ts.options.main1mBucket, startOfDayFormatted, endOfDayFormatted, ts.options.CurrentBucket),
		"origin-sample-60m": fmt.Sprintf(origin60mStringRaw, ts.options.mainBucket, startOfDayFormatted, endOfDayFormatted, ts.options.CurrentOriginBucket),
	}

	existingTasks, err := tasksAPI.FindTasks(ctx, nil)
	if err != nil {
		return err
	}

	tasks := []*influxDomain.Task{}
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

		tasks = append(tasks, task)
	}

	ts.tasks = tasks
	return nil
}

func (ts *RelayMeterTestSuite) getTestRelays() error {
	file, err := ioutil.ReadFile("../testdata/mock_relays.json")
	if err != nil {
		return err
	}
	testRelays := []TestRelay{}
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
				"host":                 "test_0bc93fa",
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

// Initializes Pocket HTTP DB with required apps and LBs (will not recreate if they already exist)
func (ts *RelayMeterTestSuite) populatePocketHTTPDB() error {
	existingApps, err := get[[]repository.Application](ts.options.phdBaseURL, "application", "", "", ts.options.phdAPIKey, ts.httpClient)
	if err != nil {
		return err
	}
	existingLBs, err := get[[]repository.LoadBalancer](ts.options.phdBaseURL, "load_balancer", "", "", ts.options.phdAPIKey, ts.httpClient)
	if err != nil {
		return err
	}
	existingAppNames := []string{}
	for _, app := range existingApps {
		existingAppNames = append(existingAppNames, app.Name)
	}
	existingLBNames := []string{}
	for _, lb := range existingLBs {
		existingLBNames = append(existingLBNames, lb.Name)
	}

	for i, application := range ts.TestRelays {
		/* Create Application -> POST /application */
		var createdAppID string
		if !stringUtils.ExactContains(existingAppNames, fmt.Sprintf("test-application-%d", i+1)) {
			appInput := fmt.Sprintf(applicationJSON, i+1, ts.options.testUserID, application.ApplicationPublicKey)
			createdApplication, err := post[repository.Application](ts.options.phdBaseURL, "application", ts.options.phdAPIKey, []byte(appInput), ts.httpClient)
			if err != nil {
				return err
			}
			createdAppID = createdApplication.ID
		}

		/* Create Load Balancer -> POST /load_balancer */
		if !stringUtils.ExactContains(existingLBNames, fmt.Sprintf("test-load-balancer-%d", i+1)) {
			loadBalancerInput := fmt.Sprintf(loadBalancerJSON, i+1, ts.options.testUserID, createdAppID)
			_, err = post[repository.LoadBalancer](ts.options.phdBaseURL, "load_balancer", ts.options.phdAPIKey, []byte(loadBalancerInput), ts.httpClient)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Manually runs the Influx tasks (takes around 30 seconds as it must wait for tasks to complete)
func (ts *RelayMeterTestSuite) runInfluxTasks() error {
	tasksAPI := ts.influxClient.TasksAPI()

	for _, task := range ts.tasks {
		_, err := tasksAPI.RunManually(ctx, task)
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Second) // Wait for task to complete
	}

	return nil
}

// POST test util func
func post[T any](baseURL, path, apiKey string, postData []byte, httpClient *httpclient.Client) (T, error) {
	var data T

	rawURL := fmt.Sprintf("%s/%s", baseURL, path)

	headers := http.Header{
		"Content-Type": {"application/json"},
		"Connection":   {"Close"},
	}
	if apiKey != "" {
		headers["Authorization"] = []string{apiKey}
	}

	postBody := bytes.NewBufferString(string(postData))

	response, err := httpClient.Post(rawURL, postBody, headers)
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

const (
	// Influx Tasks
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
	origin60mStringRaw = `from(bucket: "%s")
	|> range(start: %s, stop: %s)
	|> filter(fn: (r) => (r["_measurement"] == "origin"))
	|> filter(fn: (r) => (r["_field"] == "origin"))
	|> window(every: 1ms)
	|> reduce(fn: (r, accumulator) => ({count: accumulator.count + 1, origin: r._value}), identity: {origin: "", count: 1})
	|> to(
		bucket: "%s",
		org: "myorg",
		timeColumn: "_stop",
		tagColumns: ["origin", "applicationPublicKey"],
		fieldFn: (r) => ({"count": r.count - 1}),
	)`

	// PHD Test Data
	applicationJSON = `{
		"name": "test-application-%d",
		"userID": "%s",
		"status": "IN_SERVICE",
		"dummy": true,
		"firstDateSurpassed": null,
		"limit": {
			"payPlan": {
				"planType": "FREETIER_V0",
				"dailyLimit": 250000
			}
		},
		"gatewayAAT": {
			"address": "test_address_8dbb89278918da056f589086fb4",
			"applicationPublicKey": "%s",
			"applicationSignature": "test_key_f9c21a35787c53c8cdb532cad0dc01e099f34c28219528e3732c2da38037a84db4ce0282fa9aa404e56248155a1fbda789c8b5976711ada8588ead5",
			"clientPublicKey": "test_key_2381d403a2e2edeb37c284957fb0ee5d66f1081acb87478a5817919",
			"privateKey": "test_key_0c0fbd26d98bcbdca4d4f14fdc45ffb6db0e6a23a56671fc4b444e1b8bbd4a934adde984d117f281867cb71d9537de45473b3028ead2326cd9e27ad",
			"version": "0.0.1"
		},
		"gatewaySettings": {
			"secretKey": "test_key_ba2724be652eca0a350bc07",
			"secretKeyRequired": false
		},
			"notificationSettings": {
			"signedUp": true,
			"quarter": false,
			"half": false,
			"threeQuarters": true,
			"full": true
		}
	}`
	loadBalancerJSON = `{
		"name": "test-load-balancer-%d",
		"userID": "%s",
		"applicationIDs": ["%s"],
		"requestTimeout": 2000,
		"gigastake": false,
		"gigastakeRedirect": true,
		"stickinessOptions": {
			"duration": "",
			"stickyOrigins": null,
			"stickyMax": 0,
			"stickiness": false
		},
		"Applications": null
	}`
)
