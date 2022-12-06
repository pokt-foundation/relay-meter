package tests

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/db"
	"github.com/stretchr/testify/require"
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
