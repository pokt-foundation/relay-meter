//go:build tests

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/pokt-foundation/relay-meter/db"
	"github.com/pokt-foundation/relay-meter/tests"
	"github.com/stretchr/testify/suite"
)

var (
	ctx                      = context.Background()
	ErrResponseNotOK         = errors.New("Response not OK")
	ErrInfluxClientUnhealthy = errors.New("test influx client is unhealthy")
)

type (
	CollectorTestSuite struct {
		suite.Suite
		TestRelays   [10]tests.TestRelay
		writeAPI     api.WriteAPI
		bucketsAPI   api.BucketsAPI
		tasksAPI     api.TasksAPI
		queryAPI     api.QueryAPI
		tasks        map[string]*domain.Task
		influxSource db.Source
		orgID        string
		options      TestClientOptions
	}
	TestClientOptions struct {
		db.InfluxDBOptions
		mainBucket, main1mBucket, orgID string
	}
	Counts struct {
		Relays      int64 `json:"relays"`
		Origin      int64 `json:"origin"`
		ElapsedTime int64 `json:"elapsedTime"`
	}
	OriginCounts map[string]int64
)

// SetupSuite runs before each test suite run - takes just over 1 minute to complete
func (ts *CollectorTestSuite) SetupSuite() {
	err := ts.initInflux() // Setup Influx client to interact with Influx DB
	ts.NoError(err)
	ts.resetInfluxBuckets() // Ensure Influx buckets are empty at start of test

	err = ts.getTestRelays() // Marshals test relay JSON to array of structs
	ts.NoError(err)

	err = ts.createInfluxTasks() // Create the Influx tasks
	ts.NoError(err)

	ts.initInfluxSource() // Initialize the Influx DB Source to test contents of buckets
}

func (ts *CollectorTestSuite) SetupTest() {
	err := ts.resetInfluxBuckets()
	ts.NoError(err)
}

// Initializes the Influx client and returns it alongside the org ID
func (ts *CollectorTestSuite) initInflux() error {
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

	dOrg, err := influxClient.OrganizationsAPI().FindOrganizationByName(ctx, ts.options.Org)
	if err != nil {
		return err
	}
	ts.orgID = *dOrg.Id

	ts.bucketsAPI = influxClient.BucketsAPI()
	ts.writeAPI = influxClient.WriteAPI(ts.options.Org, ts.options.mainBucket)
	ts.tasksAPI = influxClient.TasksAPI()
	ts.queryAPI = influxClient.QueryAPI(ts.options.Org)

	return nil
}

func (ts *CollectorTestSuite) initInfluxSource() {
	ts.influxSource = db.NewInfluxDBSource(ts.options.InfluxDBOptions)
}

// Resets the Influx bucket each time the collector test runs
func (ts *CollectorTestSuite) resetInfluxBuckets() error {
	usedBuckets := []string{
		ts.options.mainBucket,
		ts.options.main1mBucket,
		ts.options.CurrentBucket,
		ts.options.CurrentOriginBucket,
	}

	for _, bucketName := range usedBuckets {
		bucket, err := ts.bucketsAPI.FindBucketByName(ctx, bucketName)
		if err == nil {
			err = ts.bucketsAPI.DeleteBucketWithID(ctx, *bucket.Id)
			if err != nil {
				return err
			}
		}
		_, err = ts.bucketsAPI.CreateBucketWithNameWithID(ctx, ts.orgID, bucketName)
		if err != nil {
			return err
		}
	}

	return nil
}

// Gets the test relays JSON from the testdata directory
func (ts *CollectorTestSuite) getTestRelays() error {
	file, err := ioutil.ReadFile("../testdata/mock_relays.json")
	if err != nil {
		return err
	}
	testRelays := [10]tests.TestRelay{}
	err = json.Unmarshal([]byte(file), &testRelays)
	if err != nil {
		return err
	}

	ts.TestRelays = testRelays
	return nil
}

// Initializes the Influx tasks used to populate each bucket from the main bucket
func (ts *CollectorTestSuite) createInfluxTasks() error {
	tasksToInit := map[string]string{
		"app-1m":            fmt.Sprintf(app1mString, ts.options.mainBucket) + fmt.Sprintf(appBucketToString, ts.options.main1mBucket),
		"app-10m":           fmt.Sprintf(app10mString, ts.options.main1mBucket) + fmt.Sprintf(appBucketToString, ts.options.CurrentBucket),
		"origin-sample-60m": fmt.Sprintf(origin60mString, ts.options.mainBucket) + fmt.Sprintf(originBucketToString, ts.options.CurrentOriginBucket),
	}

	existingTasks, err := ts.tasksAPI.FindTasks(ctx, nil)
	if err != nil {
		return err
	}

	tasks := map[string]*domain.Task{}

	for taskName, fluxTask := range tasksToInit {
		for _, existingTask := range existingTasks {
			if existingTask.Name == taskName {
				err = ts.tasksAPI.DeleteTaskWithID(ctx, existingTask.Id)
				if err != nil {
					return err
				}
			}
		}

		task, err := ts.tasksAPI.CreateTaskWithEvery(ctx, taskName, fluxTask, "1h", ts.orgID)
		if err != nil {
			return err
		}

		tasks[taskName] = task
	}

	ts.tasks = tasks

	return nil
}

// Initializes the Influx tasks used to populate each bucket from the main bucket
func (ts *CollectorTestSuite) setInfluxTasks() error {
	for taskName, fluxTask := range ts.tasks {
		updatedTask := fluxTask
		strs := strings.Split(taskName, "-")
		period := strs[len(strs)-1]
		updatedTask.Every = &period

		offsetStr := "-10s"
		updatedTask.Offset = &offsetStr

		_, err := ts.tasksAPI.UpdateTask(ctx, updatedTask)
		if err != nil {
			return err
		}
	}

	return nil
}

// Saves a test batch of relays to the InfluxDB mainBucket
func (ts *CollectorTestSuite) populateInfluxRelays(numberOfRelays int64) error {
	startTime := time.Now().Truncate(time.Minute)

	timestampInterval := (time.Minute) / time.Duration(numberOfRelays)

	for i := 0; i < int(numberOfRelays); i++ {
		relay := ts.TestRelays[i%10]
		relayTimestamp := startTime.Add(timestampInterval * time.Duration(i+1))

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
		ts.writeAPI.WritePoint(relayPoint)

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
		ts.writeAPI.WritePoint(originPoint)
	}

	ts.writeAPI.Flush()

	return nil
}

// Saves a test batch of relays to the InfluxDB mainBucket
func (ts *CollectorTestSuite) checkBucket(bucket, time string) (Counts, error) {
	counts := Counts{}
	queryString := `from(bucket: "%s")
        |> range(start: -%s)
        |> window(every: %s)`
	fluxQuery := fmt.Sprintf(queryString, bucket, time, time)

	result, err := ts.queryAPI.Query(context.Background(), fluxQuery)
	if err != nil {
		return counts, err
	}

	for result.Next() {
		field := result.Record().ValueByKey("_field")
		if field == "bytes" {
			counts.Relays++
		}
		if field == "count" {
			counts.Relays += result.Record().ValueByKey("_value").(int64)
		}
		if field == "elapsedTime" {
			counts.ElapsedTime++
		}
		if field == "origin" {
			counts.Origin++
		}
	}
	if result.Err() != nil {
		return counts, result.Err()
	}

	return counts, nil
}

// Saves a test batch of relays to the InfluxDB mainBucket
func (ts *CollectorTestSuite) checkOriginBucket() (OriginCounts, error) {
	counts := OriginCounts{}
	queryString := `from(bucket: "%s")
        |> range(start: -60m)
        |> window(every: 60m)`
	fluxQuery := fmt.Sprintf(queryString, ts.options.CurrentOriginBucket)

	result, err := ts.queryAPI.Query(context.Background(), fluxQuery)
	if err != nil {
		return counts, err
	}

	for result.Next() {
		if result.Record().ValueByKey("_measurement") == "origin" {
			origin := result.Record().ValueByKey("origin").(string)
			counts[origin] += result.Record().ValueByKey("_value").(int64)
		}
	}
	if result.Err() != nil {
		return counts, result.Err()
	}

	return counts, nil
}

func (ts *CollectorTestSuite) checkBucketTask(bucket, taskString string) (Counts, error) {
	counts := Counts{}

	result, err := ts.queryAPI.Query(context.Background(), fmt.Sprintf(taskString, bucket))
	if err != nil {
		return counts, err
	}

	for result.Next() {
		if result.Record().ValueByKey("_measurement") == "relay" {
			counts.Relays = counts.Relays + result.Record().ValueByKey("count").(int64) - 1
			counts.ElapsedTime++
		}
	}
	if result.Err() != nil {
		return counts, result.Err()
	}

	return counts, nil
}

// These Flux & JSON strings must have values filled in programmatically so must be saved here in the Go file
const (
	// Influx Tasks Flux
	app1mString = `from(bucket: "%s")
        |> range(start: -1m)
        |> filter(
            fn: (r) =>
                r._field == "elapsedTime" and (r.method != "chaincheck" and r.method != "synccheck"),
        )
        |> drop(columns: ["host", "method"])
        |> map(
            fn: (r) =>
                ({
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
        |> window(every: 1m)
        |> reduce(
            fn: (r, accumulator) =>
                ({
                    count: accumulator.count + 1,
                    total: accumulator.total + r._value,
                    elapsedTime: (accumulator.total + r._value) / float(v: accumulator.count),
                }),
            identity: {count: 1, total: 0.0, elapsedTime: 0.0},
        )`
	app10mString = `from(bucket: "%s")
        |> range(start: -10m)
        |> filter(fn: (r) => r._field == "count" or r._field == "elapsedTime")
        |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
        |> window(every: 10m)
        |> reduce(
            fn: (r, accumulator) =>
                ({
                    count: r.count + accumulator.count,
                    total: accumulator.total + float(v: r.count) * r.elapsedTime,
                    elapsedTime: (accumulator.total + r.elapsedTime) / float(v: accumulator.count),
                }),
            identity: {count: 1, total: 0.0, elapsedTime: 0.0},
        )`
	appBucketToString = `
        |> to(
            bucket: "%s",
            org: "pocket",
            timeColumn: "_stop",
            fieldFn: (r) => ({"count": r.count - 1, "elapsedTime": r.elapsedTime}),
        )`
	origin60mString = `from(bucket: "%s")
        |> range(start: -60m)
        |> filter(fn: (r) => r["_measurement"] == "origin")
        |> filter(fn: (r) => r["_field"] == "origin")
        |> window(every: 60m)
        |> reduce(
            fn: (r, accumulator) => ({count: accumulator.count + 1, origin: r._value}),
            identity: {origin: "", count: 1},
        )`
	originBucketToString = `
        |> to(
            bucket: "%s",
            org: "pocket",
            timeColumn: "_stop",
            tagColumns: ["origin", "applicationPublicKey"],
            fieldFn: (r) => ({"count": r.count - 1}),
        )`
)
