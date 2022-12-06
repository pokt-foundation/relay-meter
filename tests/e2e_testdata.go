package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/adshmh/meter/db"
	"github.com/gojektech/heimdall/httpclient"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxAPI "github.com/influxdata/influxdb-client-go/v2/api"
	influxDomain "github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/pokt-foundation/portal-api-go/repository"
	stringUtils "github.com/pokt-foundation/utils-go/strings"
	"github.com/stretchr/testify/require"
)

// TODO move this to utils-go
func startOfDay(day time.Time) time.Time {
	y, m, d := day.Date()
	l := day.Location()

	return time.Date(y, m, d, 0, 0, 0, 0, l)
}

var (
	// Test vars
	ctx               = context.Background()
	today             = startOfDay(time.Now().UTC())
	endOfDay          = today.AddDate(0, 0, 1)
	todayFormatted    = today.Format(time.RFC3339)
	endOfDayFormatted = endOfDay.Format(time.RFC3339)
	influxOptions     = db.InfluxDBOptions{
		URL:                 "http://localhost:8086",
		Token:               "mytoken",
		Org:                 "myorg",
		DailyBucket:         "mainnetRelayApp1d",
		CurrentBucket:       "mainnetRelayApp10m",
		DailyOriginBucket:   "mainnetOrigin1d",
		CurrentOriginBucket: "mainnetOrigin60m",
	}
	// Test HTTP client
	testClient = httpclient.NewClient(httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0))
	// Test errors
	errResponseNotOK = errors.New("Response not OK")

	// Selection of relays to populate the test InfluxDB (selected from this slice inside populateInfluxRelays)
	testRelays = []struct {
		applicationPublicKey,
		nodePublicKey,
		method,
		blockchain,
		blockchainSubdomain,
		origin string
		elapsedTime float64
	}{
		{
			applicationPublicKey: "12345efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c",
			nodePublicKey:        "test_node_pub_key_02fbcfbad0777942c1da5425bf0105546e7e7f53fffad9",
			method:               "eth_chainId",
			blockchain:           "49",
			blockchainSubdomain:  "fantom-mainnet",
			origin:               "https://app.test1.io",
			elapsedTime:          0.16475,
		},
		{
			applicationPublicKey: "12345e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7",
			nodePublicKey:        "test_node_pub_key_29f42f262efa514fbe93af1e963b40c5cddb2200dd9e0a",
			method:               "eth_getBalance",
			blockchain:           "49",
			blockchainSubdomain:  "fantom-mainnet",
			origin:               "https://app.test1.io",
			elapsedTime:          0.20045,
		},
		{
			applicationPublicKey: "12345244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c",
			nodePublicKey:        "test_node_pub_key_abfe693936a467d31e3246e32939ec04f5509315f39ef1",
			method:               "eth_getBlockByNumber",
			blockchain:           "49",
			blockchainSubdomain:  "fantom-mainnet",
			origin:               "https://app.test1.io",
			elapsedTime:          0.0813666666666667,
		},
		{
			applicationPublicKey: "12345166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4",
			nodePublicKey:        "test_node_pub_key_277ed47d71a69e1aef94f8338f859af46d57840060f17d",
			method:               "eth_getCode",
			blockchain:           "3",
			blockchainSubdomain:  "avax-mainnet",
			origin:               "https://app.test2.io",
			elapsedTime:          0.15785,
		},
		{
			applicationPublicKey: "12345fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53",
			nodePublicKey:        "test_node_pub_key_f64fe10e37173c0e7117490291c11e81d8851175e7ad4d",
			method:               "eth_getTransactionCount",
			blockchain:           "3",
			blockchainSubdomain:  "avax-mainnet",
			origin:               "https://app.test2.io",
			elapsedTime:          0.054675,
		},
		{
			applicationPublicKey: "12345d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69",
			nodePublicKey:        "test_node_pub_key_498b2b95ba2db09a263314e5fe23ce94fbd555d23578c1",
			method:               "eth_getStorageAt",
			blockchain:           "3",
			blockchainSubdomain:  "avax-mainnet",
			origin:               "https://app.test2.io",
			elapsedTime:          0.1093,
		},
		{
			applicationPublicKey: "123455d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55",
			nodePublicKey:        "test_node_pub_key_66b14f98b70d7e77572a6c72db611ebe28cf75d9cf542f",
			method:               "eth_blockNumber",
			blockchain:           "4",
			blockchainSubdomain:  "bsc-mainnet",
			origin:               "https://app.test3.io",
			elapsedTime:          0.2205,
		},
		{
			applicationPublicKey: "12345f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d",
			nodePublicKey:        "test_node_pub_key_b5c0fd8910bb7a121686865407e1a3ba40dcfb700d5e87",
			method:               "eth_getBalance",
			blockchain:           "4",
			blockchainSubdomain:  "bsc-mainnet",
			origin:               "https://app.test3.io",
			elapsedTime:          0.0932,
		},
		{
			applicationPublicKey: "12345019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5",
			nodePublicKey:        "test_node_pub_key_124d8dcca024f05e01fb56c91800480926d62da44b0eee",
			method:               "eth_blockNumber",
			blockchain:           "4",
			blockchainSubdomain:  "bsc-mainnet",
			origin:               "https://app.test3.io",
			elapsedTime:          0.1162,
		},
		{
			applicationPublicKey: "1234546a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d",
			nodePublicKey:        "test_node_pub_key_dbd1105f79c1c40c2fc8d8749ea5760700cff6a0504016",
			method:               "eth_blockNumber",
			blockchain:           "49",
			blockchainSubdomain:  "fantom-mainnet",
			origin:               "https://app.test1.io",
			elapsedTime:          0.0814,
		},
	}
)

const (
	// Test consts
	phdBaseURL        = "http://localhost:8090"
	apiKey            = "test_api_key_6789"
	relayMeterBaseURL = "http://localhost:9898"
	testUserID        = "12345678fgte0db3b6c63124"
	mainBucket        = "mainnetRelay"
	main1mBucket      = "mainnetRelayApp1m"

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

/* Test Utility Funcs */

// Initializes the Influx client and returns it alongside the org ID
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

// Resets the Influx bucket each time the collector test runs
func resetInfluxBuckets(client influxdb2.Client, orgID string, c *require.Assertions) {
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

// Initializes the Influx tasks used to populate each bucket from the main bucket
func initInfluxTasks(tasksAPI influxAPI.TasksAPI, orgID string, c *require.Assertions) []*influxDomain.Task {
	app1mString := fmt.Sprintf(app1mStringRaw, mainBucket, todayFormatted, endOfDayFormatted, main1mBucket)
	app10mString := fmt.Sprintf(app10mStringRaw, main1mBucket, todayFormatted, endOfDayFormatted, influxOptions.CurrentBucket)
	origin60mString := fmt.Sprintf(origin60mStringRaw, mainBucket, todayFormatted, endOfDayFormatted, influxOptions.CurrentOriginBucket)
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

// Saves a test batch of relays to the InfluxDB mainBucket
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

// Initializes Pocket HTTP DB with required apps and LBs (will not recreate if they already exist)
func populatePocketHTTPDB() error {
	existingApps, err := get[[]repository.Application](phdBaseURL, "application", "", "")
	if err != nil {
		return err
	}
	existingLBs, err := get[[]repository.LoadBalancer](phdBaseURL, "load_balancer", "", "")
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

	for i, application := range testRelays {
		/* Create Application -> POST /application */
		var createdAppID string
		if !stringUtils.ExactContains(existingAppNames, fmt.Sprintf("test-application-%d", i+1)) {
			appInput := fmt.Sprintf(applicationJSON, i+1, testUserID, application.applicationPublicKey)
			createdApplication, err := post[repository.Application](phdBaseURL, "application", []byte(appInput))
			if err != nil {
				return err
			}
			createdAppID = createdApplication.ID
		}

		/* Create Load Balancer -> POST /load_balancer */
		if !stringUtils.ExactContains(existingLBNames, fmt.Sprintf("test-load-balancer-%d", i+1)) {
			loadBalancerInput := fmt.Sprintf(loadBalancerJSON, i+1, testUserID, createdAppID)
			_, err = post[repository.LoadBalancer](phdBaseURL, "load_balancer", []byte(loadBalancerInput))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// POST test util func
func post[T any](baseURL, path string, postData []byte) (T, error) {
	var data T

	rawURL := fmt.Sprintf("%s/%s", baseURL, path)

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
		return data, fmt.Errorf("%w. %s", errResponseNotOK, http.StatusText(response.StatusCode))
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
func get[T any](baseURL, path, id, params string) (T, error) {
	rawURL := fmt.Sprintf("%s/%s", baseURL, path)
	if id != "" {
		rawURL = fmt.Sprintf("%s/%s", rawURL, id)
	}
	if params != "" {
		rawURL = fmt.Sprintf("%s%s", rawURL, params)
	}

	headers := http.Header{"Authorization": {apiKey}}

	var data T

	response, err := testClient.Get(rawURL, headers)
	if err != nil {
		return data, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return data, fmt.Errorf("%w. %s", errResponseNotOK, http.StatusText(response.StatusCode))
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
