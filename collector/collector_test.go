//go:build tests

package collector

import (
	"context"
	"database/sql"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/db"
)

func TestCollect(t *testing.T) {
	dayLayout := "2006-01-02"
	today, err := time.Parse(dayLayout, time.Now().Format(dayLayout))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	testCases := []struct {
		name               string
		maxArchiveAge      time.Duration
		firstSaved         time.Time
		lastSaved          time.Time
		expectedFrom       time.Time
		expectedTo         time.Time
		shouldCollectDaily bool
	}{
		{
			name:               "Default values for start and end",
			maxArchiveAge:      30 * 24 * time.Hour,
			shouldCollectDaily: true,
			expectedFrom:       today.Add(-30 * 24 * time.Hour),
			expectedTo:         today,
		},
		{
			name:               "Previous days with existing metrics are skipped",
			maxArchiveAge:      30 * 24 * time.Hour,
			firstSaved:         today.AddDate(0, 0, -40),
			lastSaved:          today.AddDate(0, 0, -10),
			shouldCollectDaily: true,
			expectedFrom:       today.AddDate(0, 0, -9),
			expectedTo:         today,
		},
		{
			name:          "Daily metrics are skipped altogether if yesterday's data already collected",
			maxArchiveAge: 30 * 24 * time.Hour,
			firstSaved:    today.AddDate(0, 0, -40),
			lastSaved:     today.AddDate(0, 0, -1),
		},
		{
			name:          "Today is not skipped even if metrics are saved for it",
			maxArchiveAge: 30 * 24 * time.Hour,
			firstSaved:    today.AddDate(0, 0, -40),
			lastSaved:     today,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source := &fakeSource{}
			writer := &fakeWriter{
				first: tc.firstSaved,
				last:  tc.lastSaved,
			}
			c := &collector{
				Source:        source,
				Writer:        writer,
				MaxArchiveAge: tc.maxArchiveAge,
				Logger:        logger.New(),
			}
			if err := c.collect(); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !source.todaysMetricsCollected {
				t.Errorf("Expected todays metrics to be collected.")
			}
			if !source.todaysLatencyCollected {
				t.Errorf("Expected todays latencies to be collected.")
			}

			if writer.todaysWrites != 1 {
				t.Fatalf("Expected 1 write of todays metrics, got: %d", writer.todaysWrites)
			}

			if writer.todaysLatencyWrites != 1 {
				t.Fatalf("Expected 1 write of todays latency metrics, got: %d", writer.todaysLatencyWrites)
			}

			if source.dailyMetricsCollected != tc.shouldCollectDaily {
				t.Fatalf("Expected daily metrics collection to be: %t, got: %t", tc.shouldCollectDaily, source.dailyMetricsCollected)
			}

			if !tc.shouldCollectDaily {
				return
			}

			if !source.requestedFrom.Equal(tc.expectedFrom) {
				t.Errorf("Expected 'from': %v, got: %v", tc.expectedFrom, source.requestedFrom)
			}

			if !source.requestedTo.Equal(tc.expectedTo) {
				t.Errorf("Expected 'to': %v, got: %v", tc.expectedTo, source.requestedTo)
			}
		})
	}
}

func TestStart(t *testing.T) {
	testCases := []struct {
		name             string
		collectInterval  int
		sleepDuration    time.Duration
		expectedCollects int
	}{
		{
			name:             "Data collected on every interval",
			collectInterval:  4,
			sleepDuration:    10 * time.Second,
			expectedCollects: 1 + 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source := &fakeSource{}
			writer := &fakeWriter{}
			c := &collector{
				Source:        source,
				Writer:        writer,
				MaxArchiveAge: 30 * 24 * time.Hour,
				Logger:        logger.New(),
			}

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				c.Start(ctx, tc.collectInterval, 2)
			}()
			time.Sleep(tc.sleepDuration)
			cancel()

			if writer.callsCount != tc.expectedCollects {
				t.Fatalf("Expected %d data collection calls, got: %d", tc.expectedCollects, writer.callsCount)
			}
		})
	}
}

type fakeSource struct {
	requestedFrom time.Time
	requestedTo   time.Time

	response    map[time.Time]map[string]api.RelayCounts
	responseErr error

	todaysCounts           map[string]api.RelayCounts
	todaysCountsPerOrigin  map[string]api.RelayCounts
	todaysLatency          map[string][]api.Latency
	todaysMetricsCollected bool
	dailyMetricsCollected  bool
	todaysLatencyCollected bool
}

func (f *fakeSource) DailyCounts(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error) {
	f.dailyMetricsCollected = true
	f.requestedFrom = from
	f.requestedTo = to
	return f.response, f.responseErr
}

func (f *fakeSource) TodaysCounts() (map[string]api.RelayCounts, error) {
	f.todaysMetricsCollected = true
	return f.todaysCounts, nil
}

func (f *fakeSource) TodaysCountsPerOrigin() (map[string]api.RelayCounts, error) {
	f.todaysMetricsCollected = true
	return f.todaysCountsPerOrigin, nil
}

func (f *fakeSource) TodaysLatency() (map[string][]api.Latency, error) {
	f.todaysLatencyCollected = true
	return f.todaysLatency, nil
}

type fakeWriter struct {
	first               time.Time
	last                time.Time
	callsCount          int
	todaysWrites        int
	todaysLatencyWrites int
}

func (f *fakeWriter) ExistingMetricsTimespan() (time.Time, time.Time, error) {
	f.callsCount++
	return f.first, f.last, nil
}

func (f *fakeWriter) WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error {
	return nil
}

func (f *fakeWriter) WriteTodaysMetrics(counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts, latencies map[string][]api.Latency) error {
	f.todaysWrites++
	f.todaysLatencyWrites++
	return nil
}

func (f *fakeWriter) WriteTodaysUsage(ctx context.Context, tx *sql.Tx, counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error {
	f.todaysWrites++
	return nil
}

/* Integration Tests for Influx DB */
var (
	testCtx = context.Background()

	testSuiteOptions = TestClientOptions{
		InfluxDBOptions: db.InfluxDBOptions{
			URL:                 "http://localhost:8086",
			Token:               "mytoken",
			Org:                 "pocket",
			CurrentBucket:       "mainnetRelayApp10m",
			CurrentOriginBucket: "mainnetOrigin60m",
		},
		mainBucket:   "mainnetRelay",
		main1mBucket: "mainnetRelayApp1m",
	}
)

func Test_RunSuite_InfluxIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testSuite := new(CollectorTestSuite)
	testSuite.options = testSuiteOptions
	suite.Run(t, testSuite)
}

func (ts *CollectorTestSuite) Test_TaskExecution() {
	tests := []struct {
		name        string
		numOfRelays int64
		err         error
		expectedMainCounts,
		expectedAppRelayCounts Counts
		expectedOriginCounts             OriginCounts
		expectedTodaysCounts             map[string]api.RelayCounts
		expectedSuccess, expectedFailure int64
		expectedTodaysLatency            map[string][]api.Latency
	}{
		{
			name:                   "Test contents of all buckets after sending relays",
			numOfRelays:            25,
			expectedMainCounts:     Counts{Relays: 25, Origin: 25, ElapsedTime: 25},
			expectedAppRelayCounts: Counts{Relays: 25, ElapsedTime: 13},
			expectedOriginCounts: map[string]int64{
				"https://app.test1.io": 11,
				"https://app.test2.io": 8,
				"https://app.test3.io": 6,
			},
			expectedTodaysCounts: map[string]api.RelayCounts{
				"12345019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5": {Success: 1, Failure: 1},
				"12345166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4": {Success: 3, Failure: 0},
				"12345244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c": {Success: 3, Failure: 0},
				"1234546a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d": {Success: 1, Failure: 1},
				"123455d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55": {Success: 2, Failure: 0},
				"12345d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69": {Success: 2, Failure: 0},
				"12345e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7": {Success: 3, Failure: 0},
				"12345efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c": {Success: 2, Failure: 1},
				"12345f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d": {Success: 2, Failure: 0},
				"12345fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53": {Success: 3, Failure: 0},
			},
			expectedSuccess: 22, expectedFailure: 3,
			expectedTodaysLatency: map[string][]api.Latency{},
			err:                   nil,
		},
		// {
		// 	name:                         "Test contents of all buckets after sending more relays",
		// 	numOfRelays:                  25,
		// 	expectedMainCounts:           Counts{Relays: 50, Origin: 50, ElapsedTime: 50},
		// 	expectedMainBucketTaskCounts: Counts{Relays: 50},
		// 	expectedApp1mCounts:          Counts{Relays: 50, ElapsedTime: 16},
		// 	expectedApp10mCounts:         Counts{Relays: 50, ElapsedTime: 16},
		// 	err:                          nil,
		// },
	}

	for _, test := range tests {
		err := ts.populateInfluxRelays(test.numOfRelays)
		ts.NoError(err)
		time.Sleep(3 * time.Second)

		/* Verify mainnetRelay bucket */
		mainCounts, err := ts.checkBucket(ts.options.mainBucket, "3m")
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedMainCounts, mainCounts)

		mainBucketTaskCounts, err := ts.checkBucketTask(ts.options.mainBucket, app1mString)
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedAppRelayCounts, mainBucketTaskCounts)

		/* Run app-1m task and verify mainnetRelayApp1m bucket */
		_, err = ts.tasksAPI.RunManually(testCtx, ts.tasks["app-1m"])
		ts.ErrorIs(test.err, err)
		time.Sleep(5 * time.Second)

		app1mBucketCounts, err := ts.checkBucket(ts.options.main1mBucket, "10m")
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedAppRelayCounts, app1mBucketCounts)

		app1mTaskCounts, err := ts.checkBucketTask(ts.options.main1mBucket, app10mString)
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedAppRelayCounts, app1mTaskCounts)

		/* Run app-10m task and verify mainnetRelayApp10m bucket */
		_, err = ts.tasksAPI.RunManually(testCtx, ts.tasks["app-10m"])
		ts.ErrorIs(test.err, err)
		time.Sleep(5 * time.Second)

		app10mBucketCounts, err := ts.checkBucket(ts.options.CurrentBucket, "10m")
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedAppRelayCounts, app10mBucketCounts)

		/* Run origin-sample-60m task and verify mainnetOrigin60m bucket */
		_, err = ts.tasksAPI.RunManually(testCtx, ts.tasks["origin-sample-60m"])
		ts.ErrorIs(test.err, err)
		time.Sleep(5 * time.Second)

		origin60mBucketCounts, err := ts.checkOriginBucket()
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedOriginCounts, origin60mBucketCounts)
		totalOriginCount := int64(0)
		for _, count := range origin60mBucketCounts {
			totalOriginCount += count
		}
		ts.Equal(test.expectedAppRelayCounts.Relays, totalOriginCount)

		/* Verify totals using Influx Source */
		todaysCounts, err := ts.influxSource.TodaysCounts()
		ts.ErrorIs(test.err, err)
		ts.Equal(test.expectedTodaysCounts, todaysCounts)
		var success, failure int64
		for _, count := range todaysCounts {
			success += count.Success
			failure += count.Failure
		}
		ts.Equal(test.expectedSuccess, success)
		ts.Equal(test.expectedFailure, failure)
		ts.Equal(test.expectedAppRelayCounts.Relays, success+failure)
	}
}

// func (ts *CollectorTestSuite) Test_TaskExecutionNatural() {
// 	tests := []struct {
// 		name                             string
// 		numOfRelays                      int64
// 		err                              error
// 		expectedAppRelayCounts           Counts
// 		expectedOriginCounts             OriginCounts
// 		expectedTodaysCounts             map[string]api.RelayCounts
// 		expectedSuccess, expectedFailure int64
// 		expectedTodaysLatency            map[string][]api.Latency
// 	}{
// 		{
// 			name:                   "Test contents of all buckets after sending relays",
// 			numOfRelays:            100_000,
// 			expectedAppRelayCounts: Counts{Relays: 100_000, ElapsedTime: 20},
// 			expectedOriginCounts: map[string]int64{
// 				"https://app.test1.io": 11,
// 				"https://app.test2.io": 8,
// 				"https://app.test3.io": 6,
// 			},
// 			expectedTodaysCounts: map[string]api.RelayCounts{
// 				"12345019c3f109073c77cd6d8bca9d1ff21b1ad5328ba04a5a610ba1bd72e1c5": {Success: 1, Failure: 1},
// 				"12345166969ae8263902693c0fc1d3569207a4f6f380d3d4d514e7fd819a69d4": {Success: 3, Failure: 0},
// 				"12345244e2ede722d756188316196fbf1018dbec087f6caee76bb4bc2861d46c": {Success: 3, Failure: 0},
// 				"1234546a9d0c7d69ac65ffcd508b068ea2651e5d4bd5f4760bd2643584b9ef6d": {Success: 1, Failure: 1},
// 				"123455d57036c3bb5bd0de0319771cfdb3b2a28d4d64e33a3a23e6dcd6057f55": {Success: 2, Failure: 0},
// 				"12345d9470aef46d0ebd3cb3076f3a5a3228c650e374c69b3665b0e5b0017a69": {Success: 2, Failure: 0},
// 				"12345e4c4c130d41268513268a376ed2204104b2ac4498a35d3fe26450460fb7": {Success: 3, Failure: 0},
// 				"12345efc21889053171849c02dc71c4859b113c8b7b66dd2fbde268381e07a7c": {Success: 2, Failure: 1},
// 				"12345f9fe17a1f6aaca7fe9df6499e569ae2f4910708f636beb1efa20cebda4d": {Success: 2, Failure: 0},
// 				"12345fe0fc623aff8bba4ba1984e0c521c08874c9519cc6dcd518a15dd241f53": {Success: 3, Failure: 0},
// 			},
// 			expectedSuccess: 22, expectedFailure: 3,
// 			expectedTodaysLatency: map[string][]api.Latency{},
// 			err:                   nil,
// 		},
// 	}

// 	err := ts.setInfluxTasks()
// 	ts.NoError(err)

// 	for _, test := range tests {
// 		err := ts.populateInfluxRelays(test.numOfRelays)
// 		ts.NoError(err)

// 		time.Sleep(1*time.Minute + (10 * time.Second))

// 		/* Run app-1m task and verify mainnetRelayApp1m bucket */
// 		app1mBucketCounts, err := ts.checkBucket(ts.options.main1mBucket, "10m")
// 		ts.ErrorIs(test.err, err)
// 		PrettyString("app1mBucketCounts", app1mBucketCounts)
// 		ts.Equal(test.expectedAppRelayCounts, app1mBucketCounts)

// 		app1mTaskCounts, err := ts.checkBucketTask(ts.options.main1mBucket, app10mString)
// 		ts.ErrorIs(test.err, err)
// 		PrettyString("app1mTaskCounts", app1mTaskCounts)
// 		ts.Equal(test.expectedAppRelayCounts, app1mTaskCounts)

// 		/* Run app-10m task and verify mainnetRelayApp10m bucket */
// 		// _, err = ts.tasksAPI.RunManually(testCtx, ts.tasks["app-10m"])
// 		// ts.ErrorIs(test.err, err)
// 		// time.Sleep(5 * time.Second)

// 		// app10mBucketCounts, err := ts.checkBucket(ts.options.CurrentBucket, "10m")
// 		// ts.ErrorIs(test.err, err)
// 		// PrettyString("app10mBucketCounts", app10mBucketCounts)
// 		// // ts.Equal(test.expectedAppRelayCounts, app10mBucketCounts)

// 		// /* Verify mainnetOrigin60m bucket */
// 		// origin60mBucketCounts, err := ts.checkOriginBucket()
// 		// ts.ErrorIs(test.err, err)
// 		// PrettyString("origin60mBucketCounts", origin60mBucketCounts)
// 		// ts.Equal(test.expectedOriginCounts, origin60mBucketCounts)
// 		// totalOriginCount := int64(0)
// 		// for _, count := range origin60mBucketCounts {
// 		// 	totalOriginCount += count
// 		// }
// 		// ts.Equal(test.expectedAppRelayCounts.Relays, totalOriginCount)

// 		/* Verify totals using Influx Source */
// 		// todaysCounts, err := ts.influxSource.TodaysCounts()
// 		// PrettyString("todaysCounts", todaysCounts)
// 	}
// }
