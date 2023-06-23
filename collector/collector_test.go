package collector

import (
	"context"
	"database/sql"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/pokt-foundation/relay-meter/api"
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
			source1 := &fakeSource{}
			source2 := &fakeSource{}
			sources := []*fakeSource{source1, source2}
			writer := &fakeWriter{
				first: tc.firstSaved,
				last:  tc.lastSaved,
			}
			c := &collector{
				Sources:       []Source{sources[0], sources[1]},
				Writer:        writer,
				MaxArchiveAge: tc.maxArchiveAge,
				Logger:        logger.New(),
			}
			if err := c.collect(); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			for _, source := range sources {
				if !source.todaysMetricsCollected {
					t.Errorf("Expected todays metrics to be collected.")
				}
				if !source.todaysLatencyCollected {
					t.Errorf("Expected todays latencies to be collected.")
				}
				if source.dailyMetricsCollected != tc.shouldCollectDaily {
					t.Fatalf("Expected daily metrics collection to be: %t, got: %t", tc.shouldCollectDaily, source.dailyMetricsCollected)
				}
				if !source.requestedFrom.Equal(tc.expectedFrom) {
					t.Errorf("Expected 'from': %v, got: %v", tc.expectedFrom, source.requestedFrom)
				}
				if !source.requestedTo.Equal(tc.expectedTo) {
					t.Errorf("Expected 'to': %v, got: %v", tc.expectedTo, source.requestedTo)
				}
			}
			//TODO: enable again after GCP deploy
			/* if writer.todaysWrites != 1 {
				t.Fatalf("Expected 1 write of todays metrics, got: %d", writer.todaysWrites)
			}

			if writer.todaysLatencyWrites != 1 {
				t.Fatalf("Expected 1 write of todays latency metrics, got: %d", writer.todaysLatencyWrites)
			}

			if !tc.shouldCollectDaily {
				return
			} */
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
				Sources:       []Source{source},
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

func (f *fakeSource) Name() string {
	return "fake"
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
