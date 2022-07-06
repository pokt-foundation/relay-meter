package collector

import (
	"context"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"
)

func TestCollect(t *testing.T) {
	dayLayout := "2006-01-02"
	today, err := time.Parse(dayLayout, time.Now().Format(dayLayout))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	testCases := []struct {
		name                 string
		maxArchiveAge        time.Duration
		firstSaved           time.Time
		lastSaved            time.Time
		expectedFrom         time.Time
		expectedTo           time.Time
		expectedTodaysWrites int
	}{
		{
			name:                 "Default values for start and end",
			maxArchiveAge:        30 * 24 * time.Hour,
			expectedFrom:         today.Add(-30 * 24 * time.Hour),
			expectedTo:           today.AddDate(0, 0, 1),
			expectedTodaysWrites: 1,
		},
		{
			name:                 "Previous days with existing metrics are skipped",
			maxArchiveAge:        30 * 24 * time.Hour,
			firstSaved:           today.AddDate(0, 0, -40),
			lastSaved:            today.AddDate(0, 0, -10),
			expectedFrom:         today.AddDate(0, 0, -9),
			expectedTo:           today.AddDate(0, 0, 1),
			expectedTodaysWrites: 1,
		},
		{
			name:                 "Today is not skipped even if metrics are saved for it",
			maxArchiveAge:        30 * 24 * time.Hour,
			firstSaved:           today.AddDate(0, 0, -40),
			lastSaved:            today,
			expectedFrom:         today,
			expectedTo:           today.AddDate(0, 0, 1),
			expectedTodaysWrites: 1,
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

			if !source.requestedFrom.Equal(tc.expectedFrom) {
				t.Fatalf("Expected 'from': %v, got: %v", tc.expectedFrom, source.requestedFrom)
			}

			if !source.requestedTo.Equal(tc.expectedTo) {
				t.Fatalf("Expected 'to': %v, got: %v", tc.expectedTo, source.requestedTo)
			}

			if writer.todaysWrites != tc.expectedTodaysWrites {
				t.Fatalf("Expected %d writes of todays metrics, got: %d", tc.expectedTodaysWrites, writer.todaysWrites)
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
			collectInterval:  3,
			sleepDuration:    5 * time.Second,
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
				c.Start(ctx, tc.collectInterval, 1)
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

	response    map[time.Time]map[string]int64
	responseErr error

	todaysCounts map[string]int64
}

func (f *fakeSource) DailyCounts(from, to time.Time) (map[time.Time]map[string]int64, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.response, f.responseErr
}

func (f *fakeSource) TodaysCounts() (map[string]int64, error) {
	return f.todaysCounts, nil
}

type fakeWriter struct {
	first        time.Time
	last         time.Time
	callsCount   int
	todaysWrites int
}

func (f *fakeWriter) ExistingMetricsTimespan() (time.Time, time.Time, error) {
	f.callsCount++
	return f.first, f.last, nil
}

func (f *fakeWriter) WriteDailyUsage(counts map[time.Time]map[string]int64) error {
	return nil
}

func (f *fakeWriter) WriteTodaysUsage(counts map[string]int64) error {
	f.todaysWrites++
	return nil
}
