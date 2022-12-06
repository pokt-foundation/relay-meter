package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/db"
	"github.com/gojektech/heimdall/httpclient"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxAPI "github.com/influxdata/influxdb-client-go/v2/api"
	influxDomain "github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/pokt-foundation/portal-api-go/repository"
	"github.com/stretchr/testify/require"
)

const (
	mainBucket   = "mainnetRelay"
	main1mBucket = "mainnetRelayApp1m"
)

var (
	ctx               = context.Background()
	today             = startOfDay(time.Now())
	todayFormatted    = today.AddDate(0, 0, -2).Format(time.RFC3339)
	endOfDayFormatted = today.AddDate(0, 0, 2).Format(time.RFC3339)
	influxOptions     = db.InfluxDBOptions{
		URL:                 "http://localhost:8086",
		Token:               "mytoken",
		Org:                 "myorg",
		DailyBucket:         "mainnetRelayApp1d",
		CurrentBucket:       "mainnetRelayApp10m",
		DailyOriginBucket:   "mainnetOrigin1d",
		CurrentOriginBucket: "mainnetOrigin60m",
	}
)

func Test_RelayMeter_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	c := require.New(t)

	/* Initialize PHD Data */
	err := populatePocketHTTPDB()
	c.NoError(err)

	/* Create Test Influx Client */
	client, orgID := initTestInfluxClient(c)
	tasksAPI := client.TasksAPI()
	writeAPI := client.WriteAPI(influxOptions.Org, mainBucket)

	initInfluxBuckets(client, orgID, c)
	tasks := initInfluxTasks(tasksAPI, orgID, c)

	/* Create Test Client to Verify InfluxDB contents */
	testInfluxClient := db.NewInfluxDBSource(influxOptions)

	tests := []struct {
		name                  string
		numberOfRelays        int
		expectedDailyCounts   map[time.Time]map[string]api.RelayCounts
		expectedHourlyLatency map[string]float64
		err                   error
	}{
		{
			name:           "Should collect a set number of relays from Influx",
			numberOfRelays: 100_000,
			expectedDailyCounts: map[time.Time]map[string]api.RelayCounts{
				today: {
					"test_019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5": {Success: 8889, Failure: 1111},
					"test_166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4": {Success: 8889, Failure: 1111},
					"test_244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c": {Success: 8889, Failure: 1111},
					"test_46a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d": {Success: 8888, Failure: 1111},
					"test_5d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55": {Success: 8889, Failure: 1111},
					"test_d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69": {Success: 8889, Failure: 1111},
					"test_e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7": {Success: 8889, Failure: 1111},
					"test_efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c": {Success: 8888, Failure: 1112},
					"test_f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d": {Success: 8889, Failure: 1111},
					"test_fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53": {Success: 8889, Failure: 1111},
				},
			},
			expectedHourlyLatency: map[string]float64{
				"test_019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5": 0.1162,
				"test_166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4": 0.15785,
				"test_244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c": 0.08137,
				"test_46a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d": 0.0814,
				"test_5d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55": 0.2205,
				"test_d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69": 0.1093,
				"test_e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7": 0.20045,
				"test_efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c": 0.16475,
				"test_f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d": 0.0932,
				"test_fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53": 0.05467,
			},
			err: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			/* Populate Relays in Influx DB */
			populateInfluxRelays(writeAPI, today, test.numberOfRelays)
			time.Sleep(1 * time.Second)
			for _, task := range tasks {
				_, err := tasksAPI.RunManually(ctx, task)
				c.NoError(err)
				time.Sleep(10 * time.Second) // Wait for task to complete
			}

			/* Verify Results from Influx Using Collector Influx Methods */
			dailyCounts, err := testInfluxClient.DailyCounts(today, today.AddDate(0, 0, 1))
			c.NoError(err)
			totalSuccess, totalFailure := 0, 0
			for _, count := range dailyCounts[today] {
				totalSuccess += int(count.Success)
				totalFailure += int(count.Failure)
			}
			// One relay missed due to collection interval between buckets - applies only to test
			c.Equal(test.numberOfRelays-1, totalSuccess+totalFailure)
			c.Equal(test.expectedDailyCounts, dailyCounts)

			todaysCounts, err := testInfluxClient.TodaysCounts()
			c.NoError(err)
			for i, count := range todaysCounts {
				c.NotEmpty(count.Success)
				c.NotEmpty(count.Failure)
				// Count will be for an incomplete day so less relays than Daily Count
				c.LessOrEqual(count.Success, test.expectedDailyCounts[today][i].Success)
				c.LessOrEqual(count.Failure, test.expectedDailyCounts[today][i].Failure)
			}

			todaysCountsPerOrigin, err := testInfluxClient.TodaysCountsPerOrigin()
			c.NoError(err)
			for origin, countPerOrigin := range todaysCountsPerOrigin {
				// Daily Count by Origin query does not record failures
				c.NotEmpty(countPerOrigin.Success)
				c.Contains([]string{"https://app.test1.io", "https://app.test2.io", "https://app.test3.io"}, origin)
			}

			todaysLatency, err := testInfluxClient.TodaysLatency()
			c.NoError(err)
			for app, latencies := range todaysLatency {
				for _, hourlyLatency := range latencies {
					c.NotEmpty(hourlyLatency)
					if hourlyLatency.Latency != 0 {
						c.Equal(test.expectedHourlyLatency[app], hourlyLatency.Latency)
					}
				}
			}
		})
	}

	client.Close()
}

func PrettyString(label string, thing interface{}) {
	jsonThing, _ := json.Marshal(thing)
	str := string(jsonThing)

	var prettyJSON bytes.Buffer
	_ = json.Indent(&prettyJSON, []byte(str), "", "    ")
	output := prettyJSON.String()

	fmt.Println(label, output)
}

/* Populate Influx DB */
func initTestInfluxClient(c *require.Assertions) (influxdb2.Client, string) {
	client := influxdb2.NewClientWithOptions(
		influxOptions.URL, influxOptions.Token,
		influxdb2.DefaultOptions().SetBatchSize(4_000),
	)

	dOrg, err := client.OrganizationsAPI().FindOrganizationByName(ctx, influxOptions.Org)
	c.NoError(err)

	health, err := client.Health(ctx)
	c.NoError(err)
	c.Equal("ready for queries and writes", *health.Message)

	return client, *dOrg.Id
}

func initInfluxBuckets(client influxdb2.Client, orgID string, c *require.Assertions) {
	bucketsAPI := client.BucketsAPI()
	usedBuckets := []string{mainBucket, main1mBucket, influxOptions.CurrentBucket, influxOptions.CurrentOriginBucket}

	for _, bucketName := range usedBuckets {
		bucket, err := bucketsAPI.FindBucketByName(ctx, bucketName)
		if err == nil {
			err = client.BucketsAPI().DeleteBucketWithID(ctx, *bucket.Id)
			c.NoError(err)
		}
		_, err = client.BucketsAPI().CreateBucketWithNameWithID(ctx, orgID, bucketName)
		c.NoError(err)
	}
}

func initInfluxTasks(tasksAPI influxAPI.TasksAPI, orgID string, c *require.Assertions) []*influxDomain.Task {
	app1mString := fmt.Sprintf(`from(bucket: "%s")
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
		)`, mainBucket, todayFormatted, endOfDayFormatted, main1mBucket)
	app10mString := fmt.Sprintf(`from(bucket: "%s")
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
		)`, main1mBucket, todayFormatted, endOfDayFormatted, influxOptions.CurrentBucket)
	origin60mString := fmt.Sprintf(`from(bucket: "%s")
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
		)`, mainBucket, todayFormatted, endOfDayFormatted, influxOptions.CurrentOriginBucket)
	taskNames := []string{"app-1m", "app-10m", "origin-sample-60m"}

	existingTasks, err := tasksAPI.FindTasks(ctx, nil)
	c.NoError(err)

	tasks := []*influxDomain.Task{}
	for i, fluxTask := range []string{app1mString, app10mString, origin60mString} {
		taskName := taskNames[i]

		for _, existingTask := range existingTasks {
			if existingTask.Name == taskName {
				err = tasksAPI.DeleteTaskWithID(ctx, existingTask.Id)
				c.NoError(err)
			}
		}

		task, err := tasksAPI.CreateTaskWithEvery(ctx, taskNames[i], fluxTask, "1h", orgID)
		c.NoError(err)

		tasks = append(tasks, task)
	}

	return tasks
}

// Sends a test batch of relays
func populateInfluxRelays(writeAPI influxAPI.WriteAPI, date time.Time, numberOfRelays int) {
	timestampInterval := (24 * time.Hour) / time.Duration(numberOfRelays)

	for i := 0; i < numberOfRelays; i++ {
		index := i % 10
		relay := testRelays[index]

		relayTimestamp := date.Add(timestampInterval * time.Duration(i+1))
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
				"applicationPublicKey": relay.applicationPublicKey,
				"nodePublicKey":        relay.nodePublicKey,
				"method":               relay.method,
				"result":               result,
				"blockchain":           relay.blockchain,
				"blockchainSubdomain":  relay.blockchainSubdomain,
				"host":                 "test_0bc93fa",
				"region":               "us-east-2",
			},
			map[string]interface{}{
				"bytes":       12345,
				"elapsedTime": relay.elapsedTime,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(relayPoint)

		originPoint := influxdb2.NewPoint(
			"origin",
			map[string]string{
				"applicationPublicKey": relay.applicationPublicKey,
			},
			map[string]interface{}{
				"origin": relay.origin,
			},
			relayTimestamp,
		)
		writeAPI.WritePoint(originPoint)
	}

	writeAPI.Flush()
}

type RandomRelay struct {
	applicationPublicKey,
	nodePublicKey,
	method,
	blockchain,
	blockchainSubdomain,
	origin string
	elapsedTime float64
}

// Selection of relays to populate the test InfluxDB (randomly selected from this slice inside )
var testRelays = []RandomRelay{
	{
		applicationPublicKey: "test_efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c",
		nodePublicKey:        "test_node_pub_key_02fbcfbad0777942c1da5425bf0105546e7e7f53fffad9",
		method:               "eth_chainId",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.16475,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7",
		nodePublicKey:        "test_node_pub_key_29f42f262efa514fbe93af1e963b40c5cddb2200dd9e0a",
		method:               "eth_getBalance",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.20045,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c",
		nodePublicKey:        "test_node_pub_key_abfe693936a467d31e3246e32939ec04f5509315f39ef1",
		method:               "eth_getBlockByNumber",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.0813666666666667,
		origin:               "https://app.test1.io",
	},
	{
		applicationPublicKey: "test_166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4",
		nodePublicKey:        "test_node_pub_key_277ed47d71a69e1aef94f8338f859af46d57840060f17d",
		method:               "eth_getCode",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.15785,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53",
		nodePublicKey:        "test_node_pub_key_f64fe10e37173c0e7117490291c11e81d8851175e7ad4d",
		method:               "eth_getTransactionCount",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.054675,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69",
		nodePublicKey:        "test_node_pub_key_498b2b95ba2db09a263314e5fe23ce94fbd555d23578c1",
		method:               "eth_getStorageAt",
		blockchain:           "3",
		blockchainSubdomain:  "avax-mainnet",
		elapsedTime:          0.1093,
		origin:               "https://app.test2.io",
	},
	{
		applicationPublicKey: "test_5d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55",
		nodePublicKey:        "test_node_pub_key_66b14f98b70d7e77572a6c72db611ebe28cf75d9cf542f",
		method:               "eth_blockNumber",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.2205,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d",
		nodePublicKey:        "test_node_pub_key_b5c0fd8910bb7a121686865407e1a3ba40dcfb700d5e87",
		method:               "eth_getBalance",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.0932,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5",
		nodePublicKey:        "test_node_pub_key_124d8dcca024f05e01fb56c91800480926d62da44b0eee",
		method:               "eth_blockNumber",
		blockchain:           "4",
		blockchainSubdomain:  "bsc-mainnet",
		elapsedTime:          0.1162,
		origin:               "https://app.test3.io",
	},
	{
		applicationPublicKey: "test_46a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d",
		nodePublicKey:        "test_node_pub_key_dbd1105f79c1c40c2fc8d8749ea5760700cff6a0504016",
		method:               "eth_blockNumber",
		blockchain:           "49",
		blockchainSubdomain:  "fantom-mainnet",
		elapsedTime:          0.0814,
		origin:               "https://app.test1.io",
	},
}

/* Initialize Pocket HTTP DB */
const (
	phdBaseURL = "http://localhost:8090"
	apiKey     = "test_api_key_6789"
	// connectionString = "postgres://postgres:pgpassword@localhost:5432/postgres?sslmode=disable"
	testUserID = "test_user_id_0db3b6c631c4"
)

var (
	ErrResponseNotOK error = errors.New("Response not OK")
	testClient             = httpclient.NewClient(httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0))
)

func populatePocketHTTPDB() error {
	for i, application := range testRelays {
		/* Create Application -> POST /application */
		appInput := fmt.Sprintf(applicationJSON, i+1, testUserID, application.applicationPublicKey)
		createdApplication, err := postToPHD[repository.Application]("application", []byte(appInput))
		if err != nil {
			return err
		}

		/* Create Load Balancer -> POST /load_balancer */
		loadBalancerInput := fmt.Sprintf(loadBalancerJSON, i+1, testUserID, createdApplication.ID)
		_, err = postToPHD[repository.LoadBalancer]("load_balancer", []byte(loadBalancerInput))
		if err != nil {
			return err
		}
	}

	return nil
}

// Sends a POST request to PHD to populate the test Portal DB
func postToPHD[T any](path string, postData []byte) (T, error) {
	var data T

	rawURL := fmt.Sprintf("%s/%s", phdBaseURL, path)

	headers := http.Header{
		"Authorization": {apiKey},
		"Content-Type":  {"application/json"},
		"Connection":    {"Close"},
	}

	postBody := bytes.NewBufferString(string(postData))

	response, err := testClient.Post(rawURL, postBody, headers)
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

func startOfDay(day time.Time) time.Time {
	y, m, d := day.Date()
	l := day.Location()

	return time.Date(y, m, d, 0, 0, 0, 0, l)
}
