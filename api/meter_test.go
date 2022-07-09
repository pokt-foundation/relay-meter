package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	logger "github.com/sirupsen/logrus"
)

func TestAppRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))

	testCases := []struct {
		name                string
		app                 string
		from                time.Time
		to                  time.Time
		usageData           map[time.Time]map[string]int64
		todaysUsage         map[string]int64
		expected            AppRelaysResponse
		expectedErr         error
		expectedTodaysCalls int
	}{
		{
			name: "Correct count is returned",
			app:  "app1",
			from: now.AddDate(0, 0, -5),
			to:   now.AddDate(0, 0, -1),
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
				now:                   {"app1": 1, "app2": 2},
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -5),
				To:          now,
				Count:       10,
			},
		},
		{
			name: "From and To parameters are adjusted to start of the specifed day and the next day, respectively",
			app:  "app1",
			from: time.Now().AddDate(0, 0, -5),
			to:   time.Now().AddDate(0, 0, -1),
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
				now:                   {"app1": 1, "app2": 2},
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -5),
				To:          now,
				Count:       10,
			},
		},
		{
			name: "Equal values are allowed for From and To parameters (to cover a single day)",
			app:  "app1",
			from: now.AddDate(0, 0, -3),
			to:   now.AddDate(0, 0, -3),
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -3),
				To:          now.AddDate(0, 0, -2),
				Count:       2,
			},
		},
		{
			name: "Today's metrics are added to previous days' usage data",
			app:  "app1",
			from: now.AddDate(0, 0, -3),
			to:   now,
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
			},
			todaysUsage: map[string]int64{
				"app1": 50,
				"app2": 30,
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -3),
				To:          now.AddDate(0, 0, 1),
				Count:       56,
			},
		},
		{
			name: "Only today's data is included when from and to parameters point to today",
			app:  "app1",
			from: now,
			to:   now,
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
			},
			todaysUsage: map[string]int64{
				"app1": 50,
				"app2": 30,
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now,
				To:          now.AddDate(0, 0, 1),
				Count:       50,
			},
		},
		{
			name: "Today's metrics are not included when the 'to' parameter does not include today",
			app:  "app1",
			from: time.Now().AddDate(0, 0, -3),
			to:   time.Now().AddDate(0, 0, -1),
			usageData: map[time.Time]map[string]int64{
				now.AddDate(0, 0, -6): {"app1": 1, "app2": 1},
				now.AddDate(0, 0, -5): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -4): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -3): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -2): {"app1": 2, "app2": 1},
				now.AddDate(0, 0, -1): {"app1": 2, "app2": 1},
			},
			todaysUsage: map[string]int64{
				"app1": 50,
				"app2": 30,
			},
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -3),
				To:          now,
				Count:       6,
			},
		},
		{
			name:        "Invalid timespan is rejected",
			app:         "app1",
			from:        now.AddDate(0, 0, -1),
			to:          now.AddDate(0, 0, -2),
			expectedErr: fmt.Errorf("Invalid timespan"),
			expected: AppRelaysResponse{
				Application: "app1",
				From:        now.AddDate(0, 0, -1),
				To:          now.AddDate(0, 0, -2),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       tc.usageData,
				todaysUsage: tc.todaysUsage,
			}

			relayMeter := NewRelayMeter(&fakeBackend, logger.New(), 100*time.Millisecond)
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.AppRelays(tc.app, tc.from, tc.to)
			if err != nil {
				if tc.expectedErr == nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), tc.expectedErr.Error()) {
					t.Fatalf("Expected error to contain: %q, got: %v", tc.expectedErr.Error(), err)
				}
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeBackend struct {
	usage       map[time.Time]map[string]int64
	err         error
	todaysUsage map[string]int64

	todaysMetricsCalls int
	dailyMetricsCalls  int
}

func (f *fakeBackend) DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error) {
	f.dailyMetricsCalls++
	return f.usage, f.err
}

func (f *fakeBackend) TodaysUsage() (map[string]int64, error) {
	f.todaysMetricsCalls++
	return f.todaysUsage, nil
}
