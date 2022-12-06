package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/db"
	"github.com/gojektech/heimdall/httpclient"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxAPI "github.com/influxdata/influxdb-client-go/v2/api"
	influxDomain "github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/pokt-foundation/portal-api-go/repository"
	stringUtils "github.com/pokt-foundation/utils-go/strings"
	"github.com/stretchr/testify/require"
)

const (
	phdBaseURL        = "http://localhost:8090"
	apiKey            = "test_api_key_6789"
	relayMeterBaseURL = "http://localhost:9898"
	testUserID        = "12345678fgte0db3b6c63124"
	mainBucket        = "mainnetRelay"
	main1mBucket      = "mainnetRelayApp1m"
)

var (
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

	testClient       = httpclient.NewClient(httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0))
	ErrResponseNotOK = errors.New("Response not OK")
)

func Test_RelayMeter_Collector_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	c := require.New(t)

	/* Create Test Influx Client */
	client, orgID := initTestInfluxClient(c)
	tasksAPI := client.TasksAPI()
	writeAPI := client.WriteAPI(influxOptions.Org, mainBucket)

	resetInfluxBuckets(client, orgID, c)
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
					testRelays[0].applicationPublicKey: {Success: 8888, Failure: 1112},
					testRelays[1].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[2].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[3].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[4].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[5].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[6].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[7].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[8].applicationPublicKey: {Success: 8889, Failure: 1111},
					testRelays[9].applicationPublicKey: {Success: 8888, Failure: 1111},
				},
			},
			expectedHourlyLatency: map[string]float64{
				testRelays[0].applicationPublicKey: 0.16475,
				testRelays[1].applicationPublicKey: 0.20045,
				testRelays[2].applicationPublicKey: 0.08137,
				testRelays[3].applicationPublicKey: 0.15785,
				testRelays[4].applicationPublicKey: 0.05467,
				testRelays[5].applicationPublicKey: 0.1093,
				testRelays[6].applicationPublicKey: 0.2205,
				testRelays[7].applicationPublicKey: 0.0932,
				testRelays[8].applicationPublicKey: 0.1162,
				testRelays[9].applicationPublicKey: 0.0814,
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

func Test_RelayMeter_APIServer_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end to end test")
	}

	c := require.New(t)

	/* Initialize PHD Data */
	err := populatePocketHTTPDB()
	c.NoError(err)

	time.Sleep(30 * time.Second) // Wait for collector to run and write to Postgres

	tests := []struct {
		name string
		date time.Time
		appPubKey,
		userID,
		origin string
		expectedHourlyLatency map[string]float64
		err                   error
	}{
		{
			name:      "Should return relays from the API server",
			date:      today,
			appPubKey: testRelays[0].applicationPublicKey,
			userID:    testUserID,
			origin:    testRelays[0].origin,
			expectedHourlyLatency: map[string]float64{
				testRelays[0].applicationPublicKey: 0.16475,
				testRelays[1].applicationPublicKey: 0.20045,
				testRelays[2].applicationPublicKey: 0.08137,
				testRelays[3].applicationPublicKey: 0.15785,
				testRelays[4].applicationPublicKey: 0.05467,
				testRelays[5].applicationPublicKey: 0.1093,
				testRelays[6].applicationPublicKey: 0.2205,
				testRelays[7].applicationPublicKey: 0.0932,
				testRelays[8].applicationPublicKey: 0.1162,
				testRelays[9].applicationPublicKey: 0.0814,
			},
			err: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			/* Test API Server */
			dateParams := fmt.Sprintf("?from=%s&to=%s", test.date.Format(time.RFC3339), test.date.Format(time.RFC3339))

			/* /relays */
			allRelays, err := get[api.TotalRelaysResponse](relayMeterBaseURL, "v0/relays", "", dateParams)
			c.Equal(test.err, err)
			c.NotEmpty(allRelays.Count.Success)
			c.NotEmpty(allRelays.Count.Failure)
			c.Equal(allRelays.From, test.date)
			c.Equal(allRelays.To, test.date.AddDate(0, 0, 1))

			/* /relays/apps */
			allAppsRelays, err := get[[]api.AppRelaysResponse](relayMeterBaseURL, "v0/relays/apps", "", dateParams)
			c.Equal(test.err, err)
			for _, appRelays := range allAppsRelays {
				c.Len(appRelays.Application, 64)
				c.NotEmpty(appRelays.Count.Success)
				c.NotEmpty(appRelays.Count.Failure)
				c.Equal(appRelays.From, test.date)
				c.Equal(appRelays.To, test.date.AddDate(0, 0, 1))
			}

			/* /relays/apps/{APP_PUB_KEY} */
			appRelays, err := get[api.AppRelaysResponse](relayMeterBaseURL, "v0/relays/apps", test.appPubKey, dateParams)
			c.Equal(test.err, err)
			c.Len(appRelays.Application, 64)
			c.NotEmpty(appRelays.Count.Success)
			c.NotEmpty(appRelays.Count.Failure)
			c.Equal(appRelays.From, test.date)
			c.Equal(appRelays.To, test.date.AddDate(0, 0, 1))

			/* /relays/users/{USER_ID} */
			userRelays, err := get[api.UserRelaysResponse](relayMeterBaseURL, "v0/relays/users", test.userID, dateParams)
			c.Equal(test.err, err)
			c.Len(userRelays.User, 24)
			c.Len(userRelays.Applications, 10)
			c.Len(userRelays.Applications[0], 64)
			c.NotEmpty(userRelays.Count.Success)
			c.NotEmpty(userRelays.Count.Failure)
			c.Equal(userRelays.From, test.date)
			c.Equal(userRelays.To, test.date.AddDate(0, 0, 1))

			/* /relays/endpoints */
			allEndpointsRelays, err := get[[]api.LoadBalancerRelaysResponse](relayMeterBaseURL, "v0/relays/endpoints", "", dateParams)
			c.Equal(test.err, err)
			for _, endpointRelays := range allEndpointsRelays {
				c.Len(endpointRelays.Endpoint, 24)
				c.Len(endpointRelays.Applications, 1)
				c.Len(endpointRelays.Applications[0], 64)
				c.NotEmpty(endpointRelays.Count.Success)
				c.NotEmpty(endpointRelays.Count.Failure)
				c.Equal(endpointRelays.From, test.date)
				c.Equal(endpointRelays.To, test.date.AddDate(0, 0, 1))
			}

			/* /relays/endpoints/{ENDPOINT_ID} */
			endpointRelays, err := get[api.LoadBalancerRelaysResponse](relayMeterBaseURL, "v0/relays/endpoints", allEndpointsRelays[0].Endpoint, dateParams)
			c.Equal(test.err, err)
			c.Len(endpointRelays.Endpoint, 24)
			c.Len(endpointRelays.Applications, 1)
			c.Len(endpointRelays.Applications[0], 64)
			c.NotEmpty(endpointRelays.Count.Success)
			c.NotEmpty(endpointRelays.Count.Failure)
			c.Equal(endpointRelays.From, test.date)
			c.Equal(endpointRelays.To, test.date.AddDate(0, 0, 1))

			/* /relays/origin-classification */
			allOriginRelays, err := get[[]api.OriginClassificationsResponse](relayMeterBaseURL, "v0/relays/origin-classification", "", dateParams)
			c.Equal(test.err, err)
			for _, originRelays := range allOriginRelays {
				c.Len(originRelays.Origin, 20)
				c.NotEmpty(originRelays.Count.Success)
				c.Equal(originRelays.From, test.date)
				c.Equal(originRelays.To, test.date.AddDate(0, 0, 1))
			}

			/* /relays/origin-classification/{ORIGIN} */
			url, err := url.Parse(test.origin)
			c.Equal(test.err, err)
			originRelays, err := get[api.OriginClassificationsResponse](relayMeterBaseURL, "v0/relays/origin-classification", url.Host, dateParams)
			c.Equal(test.err, err)
			c.Equal(url.Host, originRelays.Origin)
			c.Len(originRelays.Origin, 12)
			c.NotEmpty(originRelays.Count.Success)
			c.Equal(originRelays.From, test.date)
			c.Equal(originRelays.To, test.date.AddDate(0, 0, 1))

			/* /latency/apps */
			allAppLatencies, err := get[[]api.AppLatencyResponse](relayMeterBaseURL, "v0/latency/apps", "", dateParams)
			c.Equal(test.err, err)
			for _, appLatency := range allAppLatencies {
				c.Len(appLatency.DailyLatency, 24)
				for _, hourlyLatency := range appLatency.DailyLatency {
					c.NotEmpty(hourlyLatency)
					if hourlyLatency.Latency != 0 {
						c.Equal(test.expectedHourlyLatency[appLatency.Application], hourlyLatency.Latency)
					}
				}
				c.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
				c.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
				c.Len(appLatency.Application, 64)
			}

			/* /latency/apps/{APP_PUB_KEY} */
			appLatency, err := get[api.AppLatencyResponse](relayMeterBaseURL, "v0/latency/apps", test.appPubKey, dateParams)
			c.Equal(test.err, err)
			c.Len(appLatency.DailyLatency, 24)
			for _, hourlyLatency := range appLatency.DailyLatency {
				c.NotEmpty(hourlyLatency)
				if hourlyLatency.Latency != 0 {
					c.Equal(test.expectedHourlyLatency[appLatency.Application], hourlyLatency.Latency)
				}
			}
			c.Equal(appLatency.From, time.Now().UTC().Add(-23*time.Hour).Truncate(time.Hour))
			c.Equal(appLatency.To, time.Now().UTC().Truncate(time.Hour))
			c.Len(appLatency.Application, 64)
		})
	}
}

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

// TODO move this to utils-go
func startOfDay(day time.Time) time.Time {
	y, m, d := day.Date()
	l := day.Location()

	return time.Date(y, m, d, 0, 0, 0, 0, l)
}
