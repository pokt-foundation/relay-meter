package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	logger "github.com/sirupsen/logrus"

	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/utils-go/numbers"
)

func TestUserRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()

	testCases := []struct {
		name     string
		user     types.UserID
		from     time.Time
		to       time.Time
		expected UserRelaysResponse
	}{
		{
			name: "Correct summary for a user",
			user: "user1",
			from: now.AddDate(0, 0, -6),
			to:   now,
			expected: UserRelaysResponse{
				From:       now.AddDate(0, 0, -6),
				To:         now.AddDate(0, 0, 1),
				User:       "user1",
				PublicKeys: []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 6*(2+1) + 50 + 30,
					Failure: 6*(3+5) + 40 + 70,
				},
			},
		},
		{
			name: "Correct summary for a user excluding today",
			user: "user1",
			from: now.AddDate(0, 0, -6),
			to:   now.AddDate(0, 0, -2),
			expected: UserRelaysResponse{
				From:       now.AddDate(0, 0, -6),
				To:         now.AddDate(0, 0, -1),
				User:       "user1",
				PublicKeys: []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 5 * (2 + 1),
					Failure: 5 * (3 + 5),
				},
			},
		},
		{
			name: "Correct summary for a user on todays metrics",
			user: "user1",
			from: now,
			to:   now,
			expected: UserRelaysResponse{
				From:       now,
				To:         now.AddDate(0, 0, 1),
				User:       "user1",
				PublicKeys: []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 50 + 30,
					Failure: 40 + 70,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
				userApps: map[types.UserID][]types.PortalAppPublicKey{
					"user1": {"app1", "app2", "app3"},
					"user2": {"app4", "app5", "app6"},
				},
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.UserRelays(context.Background(), tc.user, tc.from, tc.to)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTotalRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()

	testCases := []struct {
		name     string
		from     time.Time
		to       time.Time
		expected TotalRelaysResponse
	}{
		{
			name: "Correct summary for the network",
			from: now.AddDate(0, 0, -6),
			to:   now,
			expected: TotalRelaysResponse{
				From: now.AddDate(0, 0, -6),
				To:   now.AddDate(0, 0, 1),
				Count: RelayCounts{
					Success: 6*(2+1+5) + 50 + 30 + 500,
					Failure: 6*(3+5+7) + 40 + 70 + 700,
				},
			},
		},
		{
			name: "Correct summary for the network excluding today",
			from: now.AddDate(0, 0, -6),
			to:   now.AddDate(0, 0, -2),
			expected: TotalRelaysResponse{
				From: now.AddDate(0, 0, -6),
				To:   now.AddDate(0, 0, -1),
				Count: RelayCounts{
					Success: 5 * (2 + 1 + 5),
					Failure: 5 * (3 + 5 + 7),
				},
			},
		},
		{
			name: "Correct summary for the network on todays metrics",
			from: now,
			to:   now,
			expected: TotalRelaysResponse{
				From: now,
				To:   now.AddDate(0, 0, 1),
				Count: RelayCounts{
					Success: 50 + 30 + 500,
					Failure: 40 + 70 + 700,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.TotalRelays(context.Background(), tc.from, tc.to)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAppRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()
	requestedApp := types.PortalAppPublicKey("app1")

	testCases := []struct {
		name                string
		from                time.Time
		to                  time.Time
		usageData           map[time.Time]map[types.PortalAppPublicKey]int64
		todaysUsage         map[types.PortalAppPublicKey]int64
		expected            AppRelaysResponse
		expectedErr         error
		expectedTodaysCalls int
	}{
		{
			name: "Correct count is returned",
			from: now.AddDate(0, 0, -5),
			to:   now.AddDate(0, 0, -1),
			expected: AppRelaysResponse{
				PublicKey: requestedApp,
				From:      now.AddDate(0, 0, -5),
				To:        now,
				Count: RelayCounts{
					Success: 5 * 2,
					Failure: 5 * 3,
				},
			},
		},
		{
			name: "From and To parameters are adjusted to start of the specifed day and the next day, respectively",
			from: time.Now().AddDate(0, 0, -5),
			to:   time.Now().AddDate(0, 0, -1),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -5),
				To:        now,
				Count: RelayCounts{
					Success: 5 * 2,
					Failure: 5 * 3,
				},
			},
		},
		{
			name: "Equal values are allowed for From and To parameters (to cover a single day)",
			from: now.AddDate(0, 0, -3),
			to:   now.AddDate(0, 0, -3),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -3),
				To:        now.AddDate(0, 0, -2),
				Count: RelayCounts{
					Success: 1 * 2,
					Failure: 1 * 3,
				},
			},
		},
		{
			name: "Today's metrics are added to previous days' usage data",
			from: now.AddDate(0, 0, -3),
			to:   now,
			todaysUsage: map[types.PortalAppPublicKey]int64{
				"app1": 50,
				"app2": 30,
			},
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -3),
				To:        now.AddDate(0, 0, 1),
				Count: RelayCounts{
					Success: 3*2 + 50,
					Failure: 3*3 + 40,
				},
			},
		},
		{
			name: "Only today's data is included when from and to parameters point to today",
			from: now,
			to:   now,
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now,
				To:        now.AddDate(0, 0, 1),
				Count: RelayCounts{
					Success: 50,
					Failure: 40,
				},
			},
		},
		{
			name: "Today's metrics are not included when the 'to' parameter does not include today",
			from: time.Now().AddDate(0, 0, -3),
			to:   time.Now().AddDate(0, 0, -1),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -3),
				To:        now,
				Count: RelayCounts{
					Success: 3 * 2,
					Failure: 3 * 3,
				},
			},
		},
		{
			name: "Today's metrics are included when the 'to' parameter is after today",
			from: time.Now().AddDate(0, 0, -3),
			to:   time.Now().AddDate(0, 0, 2),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -3),
				To:        now.AddDate(0, 0, 3),
				Count: RelayCounts{
					Success: 3*2 + 50,
					Failure: 3*3 + 40,
				},
			},
		},
		{
			name: "Only today's metrics are included when the timespan only includes today",
			from: time.Now(),
			to:   time.Now().AddDate(0, 0, 2),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now,
				To:        now.AddDate(0, 0, 3),
				Count: RelayCounts{
					Success: 50,
					Failure: 40,
				},
			},
		},
		{
			name: "Missing parameters' default values",
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -30),
				To:        now.AddDate(0, 0, 1),
				Count: RelayCounts{
					Success: 6*2 + 50,
					Failure: 6*3 + 40,
				},
			},
		},
		{
			name:        "Invalid timespan is rejected",
			from:        now.AddDate(0, 0, -1),
			to:          now.AddDate(0, 0, -2),
			expectedErr: fmt.Errorf("Invalid timespan"),
			expected: AppRelaysResponse{
				PublicKey: "app1",
				From:      now.AddDate(0, 0, -1),
				To:        now.AddDate(0, 0, -2),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.AppRelays(context.Background(), requestedApp, tc.from, tc.to)
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

func TestAllAppsRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()

	testCases := []struct {
		name                string
		from                time.Time
		to                  time.Time
		usageData           map[time.Time]map[types.PortalAppPublicKey]int64
		todaysUsage         map[types.PortalAppPublicKey]int64
		expected            map[types.PortalAppPublicKey]AppRelaysResponse
		expectedErr         error
		expectedTodaysCalls int
	}{
		{
			name: "Correct count is returned",
			from: now.AddDate(0, 0, -5),
			to:   now.AddDate(0, 0, -1),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 10,
						Failure: 15,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 5,
						Failure: 25,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 25,
						Failure: 35,
					},
				},
			},
		},
		{
			name: "From and To parameters are adjusted to start of the specifed day and the next day, respectively",
			from: time.Now().AddDate(0, 0, -5),
			to:   time.Now().AddDate(0, 0, -1),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 10,
						Failure: 15,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 5,
						Failure: 25,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -5),
					To:        now,
					Count: RelayCounts{
						Success: 25,
						Failure: 35,
					},
				},
			},
		},
		{
			name: "Equal values are allowed for From and To parameters (to cover a single day)",
			from: now.AddDate(0, 0, -3),
			to:   now.AddDate(0, 0, -3),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, -2),
					Count: RelayCounts{
						Success: 2,
						Failure: 3,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, -2),
					Count: RelayCounts{
						Success: 1,
						Failure: 5,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, -2),
					Count: RelayCounts{
						Success: 5,
						Failure: 7,
					},
				},
			},
		},
		{
			name: "Today's metrics are added to previous days' usage data",
			from: now.AddDate(0, 0, -3),
			to:   now,
			todaysUsage: map[types.PortalAppPublicKey]int64{
				"app1": 50,
				"app2": 30,
			},
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 56,
						Failure: 49,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 33,
						Failure: 85,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 515,
						Failure: 721,
					},
				},
			},
		},
		{
			name: "Only today's data is included when from and to parameters point to today",
			from: now,
			to:   now,
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now,
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 50,
						Failure: 40,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now,
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 30,
						Failure: 70,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now,
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
		{
			name: "Today's metrics are not included when the 'to' parameter does not include today",
			from: time.Now().AddDate(0, 0, -3),
			to:   time.Now().AddDate(0, 0, -1),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -3),
					To:        now,
					Count: RelayCounts{
						Success: 6,
						Failure: 9,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -3),
					To:        now,
					Count: RelayCounts{
						Success: 3,
						Failure: 15,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -3),
					To:        now,
					Count: RelayCounts{
						Success: 15,
						Failure: 21,
					},
				},
			},
		},
		{
			name: "Today's metrics are included when the 'to' parameter is after today",
			from: time.Now().AddDate(0, 0, -3),
			to:   time.Now().AddDate(0, 0, 2),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 56,
						Failure: 49,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 33,
						Failure: 85,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -3),
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 515,
						Failure: 721,
					},
				},
			},
		},
		{
			name: "Only today's metrics are included when the timespan only includes today",
			from: time.Now(),
			to:   time.Now().AddDate(0, 0, 2),
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now,
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 50,
						Failure: 40,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now,
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 30,
						Failure: 70,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now,
					To:        now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
		{
			name: "Missing parameters' default values",
			expected: map[types.PortalAppPublicKey]AppRelaysResponse{
				"app1": {
					PublicKey: "app1",
					From:      now.AddDate(0, 0, -30),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 62,
						Failure: 58,
					},
				},
				"app2": {
					PublicKey: "app2",
					From:      now.AddDate(0, 0, -30),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 36,
						Failure: 100,
					},
				},
				"app4": {
					PublicKey: "app4",
					From:      now.AddDate(0, 0, -30),
					To:        now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 530,
						Failure: 742,
					},
				},
			},
		},
		{
			name:        "Invalid timespan is rejected",
			from:        now.AddDate(0, 0, -1),
			to:          now.AddDate(0, 0, -2),
			expectedErr: fmt.Errorf("Invalid timespan"),
			expected:    map[types.PortalAppPublicKey]AppRelaysResponse{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			rawGot, err := relayMeter.AllAppsRelays(context.Background(), tc.from, tc.to)
			if err != nil {
				if tc.expectedErr == nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), tc.expectedErr.Error()) {
					t.Fatalf("Expected error to contain: %q, got: %v", tc.expectedErr.Error(), err)
				}
			}

			// Need to convert it to map to be able to compare
			got := make(map[types.PortalAppPublicKey]AppRelaysResponse, len(rawGot))

			for _, relResp := range rawGot {
				got[relResp.PublicKey] = relResp
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAppLatency(t *testing.T) {
	todaysLatency := fakeTodaysLatency()
	errBackendFailure := errors.New("backend error")

	testCases := []struct {
		name                string
		requestedApp        types.PortalAppPublicKey
		expected            AppLatencyResponse
		backendErr          error
		expectedErr         error
		expectedTodaysCalls int
	}{
		{
			name:         "Correct latency data is returned",
			requestedApp: "app1",
			expected: AppLatencyResponse{
				PublicKey:    "app1",
				From:         todaysLatency["app1"][0].Time,
				To:           todaysLatency["app1"][len(todaysLatency["app1"])-1].Time,
				DailyLatency: todaysLatency["app1"],
			},
		},
		{
			name:         "Returns error if requested app not found",
			requestedApp: "app42",
			expectedErr:  ErrAppLatencyNotFound,
		},
		{
			name:         "Backend service error",
			requestedApp: "app1",
			backendErr:   errBackendFailure,
			expectedErr:  ErrAppLatencyNotFound,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				todaysLatency: todaysLatency,
				err:           tc.backendErr,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.AppLatency(context.Background(), tc.requestedApp)
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

func TestAllAppsLatencies(t *testing.T) {
	todaysLatency := fakeTodaysLatency()
	errBackendFailure := errors.New("backend error")

	testCases := []struct {
		name                string
		todaysLatency       map[types.PortalAppPublicKey][]Latency
		expected            map[types.PortalAppPublicKey]AppLatencyResponse
		emptyLatencySlice   bool
		expectedErr         error
		backendErr          error
		expectedTodaysCalls int
	}{
		{
			name: "Correct latency data is returned",
			expected: map[types.PortalAppPublicKey]AppLatencyResponse{
				"app1": {
					PublicKey:    "app1",
					From:         todaysLatency["app1"][0].Time,
					To:           todaysLatency["app1"][len(todaysLatency["app1"])-1].Time,
					DailyLatency: todaysLatency["app1"],
				},
				"app2": {
					PublicKey:    "app2",
					From:         todaysLatency["app2"][0].Time,
					To:           todaysLatency["app2"][len(todaysLatency["app2"])-1].Time,
					DailyLatency: todaysLatency["app2"],
				},
				"app4": {
					PublicKey:    "app4",
					From:         todaysLatency["app4"][0].Time,
					To:           todaysLatency["app4"][len(todaysLatency["app4"])-1].Time,
					DailyLatency: todaysLatency["app4"],
				},
			},
		},
		{
			name:        "Backend service error",
			backendErr:  errBackendFailure,
			expectedErr: errBackendFailure,
			expected:    map[types.PortalAppPublicKey]AppLatencyResponse{},
		},
		{
			name:              "Empty latency response",
			backendErr:        nil,
			expectedErr:       nil,
			emptyLatencySlice: true,
			expected:          map[types.PortalAppPublicKey]AppLatencyResponse{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.emptyLatencySlice == true {
				for key := range todaysLatency {
					todaysLatency[key] = []Latency{}
				}
			}

			fakeBackend := fakeBackend{
				todaysLatency: todaysLatency,
				err:           tc.backendErr,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			rawGot, err := relayMeter.AllAppsLatencies(context.Background())

			if err != nil {
				if tc.expectedErr == nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), tc.expectedErr.Error()) {
					t.Fatalf("Expected error to contain: %q, got: %v", tc.expectedErr.Error(), err)
				}
			}

			// Need to convert it to map to be able to compare
			got := make(map[types.PortalAppPublicKey]AppLatencyResponse, len(rawGot))

			for _, relResp := range rawGot {
				got[relResp.PublicKey] = relResp
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPortalAppRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()
	errBackendFailure := errors.New("backend error")

	testCases := []struct {
		name       string
		portalApp  types.PortalAppID
		from       time.Time
		to         time.Time
		backendErr error

		expected    PortalAppRelaysResponse
		expectedErr error
	}{
		{
			name:        "PortalApp not found error",
			expectedErr: ErrPortalAppNotFound,
		},
		{
			name:        "Backend service error",
			backendErr:  errBackendFailure,
			expectedErr: errBackendFailure,
		},
		{
			name:      "Correct summary for a portalApp",
			portalApp: "portal_app_1",
			from:      now.AddDate(0, 0, -6),
			to:        now,
			expected: PortalAppRelaysResponse{
				From:        now.AddDate(0, 0, -6),
				To:          now.AddDate(0, 0, 1),
				PortalAppID: "portal_app_1",
				PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 6*(2+1) + 50 + 30,
					Failure: 6*(3+5) + 40 + 70,
				},
			},
		},
		{
			name:      "Correct summary for a portalApp excluding today",
			portalApp: "portal_app_1",
			from:      now.AddDate(0, 0, -6),
			to:        now.AddDate(0, 0, -2),
			expected: PortalAppRelaysResponse{
				From:        now.AddDate(0, 0, -6),
				To:          now.AddDate(0, 0, -1),
				PortalAppID: "portal_app_1",
				PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 5 * (2 + 1),
					Failure: 5 * (3 + 5),
				},
			},
		},
		{
			name:      "Correct summary for a portalApp on todays metrics",
			portalApp: "portal_app_1",
			from:      now,
			to:        now,
			expected: PortalAppRelaysResponse{
				From:        now,
				To:          now.AddDate(0, 0, 1),
				PortalAppID: "portal_app_1",
				PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
				Count: RelayCounts{
					Success: 50 + 30,
					Failure: 40 + 70,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
				portalApps: map[types.PortalAppID]*types.PortalApp{
					"portal_app_1": {
						AATs: map[types.ProtocolAppID]types.AAT{
							"app1": {PublicKey: "app1"},
							"app2": {PublicKey: "app2"},
							"app3": {PublicKey: "app3"},
						},
					},
					"portal_app_2": {
						AATs: map[types.ProtocolAppID]types.AAT{
							"app4": {PublicKey: "app4"},
							"app5": {PublicKey: "app5"},
							"app6": {PublicKey: "app6"},
						},
					},
				},
				err: tc.backendErr,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			got, err := relayMeter.PortalAppRelays(context.Background(), tc.portalApp, tc.from, tc.to)
			if err != nil && !errors.Is(err, tc.expectedErr) {
				t.Fatalf("Expected error: %v, got: %v", tc.expectedErr, err)
			}

			got.PublicKeys = sortPublicKeys(got.PublicKeys)

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAllPortalAppsRelays(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	usageData := fakeDailyMetrics()
	todaysUsage := fakeTodaysMetrics()
	errBackendFailure := errors.New("backend error")

	testCases := []struct {
		name       string
		from       time.Time
		to         time.Time
		backendErr error

		expected    map[types.PortalAppID]PortalAppRelaysResponse
		expectedErr error
	}{
		{
			name:        "Backend service error",
			backendErr:  errBackendFailure,
			expectedErr: errBackendFailure,
			expected:    map[types.PortalAppID]PortalAppRelaysResponse{},
		},
		{
			name: "Correct summary for portalApps",
			from: now.AddDate(0, 0, -6),
			to:   now,
			expected: map[types.PortalAppID]PortalAppRelaysResponse{
				"portal_app_1": {
					From:        now.AddDate(0, 0, -6),
					To:          now.AddDate(0, 0, 1),
					PortalAppID: "portal_app_1",
					PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
					Count: RelayCounts{
						Success: 98,
						Failure: 158,
					},
				},
				"portal_app_2": {
					From:        now.AddDate(0, 0, -6),
					To:          now.AddDate(0, 0, 1),
					PortalAppID: "portal_app_2",
					PublicKeys:  []types.PortalAppPublicKey{"app4", "app5", "app6"},
					Count: RelayCounts{
						Success: 530,
						Failure: 742,
					},
				},
			},
		},
		{
			name: "Correct summary for portalApps excluding today",
			from: now.AddDate(0, 0, -6),
			to:   now.AddDate(0, 0, -2),
			expected: map[types.PortalAppID]PortalAppRelaysResponse{
				"portal_app_1": {
					From:        now.AddDate(0, 0, -6),
					To:          now.AddDate(0, 0, -1),
					PortalAppID: "portal_app_1",
					PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
					Count: RelayCounts{
						Success: 15,
						Failure: 40,
					},
				},
				"portal_app_2": {
					From:        now.AddDate(0, 0, -6),
					To:          now.AddDate(0, 0, -1),
					PortalAppID: "portal_app_2",
					PublicKeys:  []types.PortalAppPublicKey{"app4", "app5", "app6"},
					Count: RelayCounts{
						Success: 25,
						Failure: 35,
					},
				},
			},
		},
		{
			name: "Correct summary for portalApps on todays metrics",
			from: now,
			to:   now,
			expected: map[types.PortalAppID]PortalAppRelaysResponse{
				"portal_app_1": {
					From:        now,
					To:          now.AddDate(0, 0, 1),
					PortalAppID: "portal_app_1",
					PublicKeys:  []types.PortalAppPublicKey{"app1", "app2", "app3"},
					Count: RelayCounts{
						Success: 80,
						Failure: 110,
					},
				},
				"portal_app_2": {
					From:        now,
					To:          now.AddDate(0, 0, 1),
					PortalAppID: "portal_app_2",
					PublicKeys:  []types.PortalAppPublicKey{"app4", "app5", "app6"},
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				usage:       usageData,
				todaysUsage: todaysUsage,
				portalApps: map[types.PortalAppID]*types.PortalApp{
					"portal_app_1": {
						ID: "portal_app_1",
						AATs: map[types.ProtocolAppID]types.AAT{
							"app1": {PublicKey: "app1"},
							"app2": {PublicKey: "app2"},
							"app3": {PublicKey: "app3"},
						},
					},
					"portal_app_2": {
						ID: "portal_app_2",
						AATs: map[types.ProtocolAppID]types.AAT{
							"app4": {PublicKey: "app4"},
							"app5": {PublicKey: "app5"},
							"app6": {PublicKey: "app6"},
						},
					},
				},
				err: tc.backendErr,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			rawGot, err := relayMeter.AllPortalAppsRelays(context.Background(), tc.from, tc.to)
			if err != nil && !errors.Is(err, tc.expectedErr) {
				t.Fatalf("Expected error: %v, got: %v", tc.expectedErr, err)
			}

			// Need to convert it to map to be able to compare
			got := make(map[types.PortalAppID]PortalAppRelaysResponse, len(rawGot))

			for _, relResp := range rawGot {
				sort.Slice(relResp.PublicKeys, func(i, j int) bool {
					return relResp.PublicKeys[i] < relResp.PublicKeys[j]
				})
				got[relResp.PortalAppID] = relResp
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStartDataLoader(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))

	testCases := []struct {
		name          string
		maxArchiveAge time.Duration
		expectedFrom  time.Time
		expectedTo    time.Time
	}{
		{
			name:         "Default maxArchiveAge is applied",
			expectedFrom: now.AddDate(0, 0, -1*MAX_PAST_DAYS_METRICS_DEFAULT_DAYS),
			expectedTo:   now,
		},
		{
			name:          "The specified maxArchiveAge takes effect",
			maxArchiveAge: 24 * 5 * time.Hour,
			expectedFrom:  now.AddDate(0, 0, -5),
			expectedTo:    now,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeBackend := fakeBackend{}
			meter := &relayMeter{
				Backend: &fakeBackend,
				Logger:  logger.New(),
				RelayMeterOptions: RelayMeterOptions{
					LoadInterval: 1 * time.Second,
					MaxPastDays:  tc.maxArchiveAge,
				},
			}

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				meter.StartDataLoader(ctx)
			}()
			// We only need the initial data loader run
			time.Sleep(10 * time.Millisecond)
			cancel()

			if !tc.expectedFrom.Equal(fakeBackend.dailyMetricsFrom) {
				t.Errorf("Expected 'from' to be: %v, got: %v", tc.expectedFrom, fakeBackend.dailyMetricsFrom)
			}

		})
	}
}

func TestAllRelaysOrigin(t *testing.T) {
	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	todaysUsage := fakeTodaysMetricsByOrigin()

	testCases := []struct {
		name                string
		from                time.Time
		to                  time.Time
		todaysOriginUsage   map[types.PortalAppOrigin]int64
		expected            map[types.PortalAppOrigin]OriginClassificationsResponse
		expectedErr         error
		expectedTodaysCalls int
	}{
		{
			name: "Only today's data is included when from and to parameters point to today",
			from: now,
			to:   now,
			expected: map[types.PortalAppOrigin]OriginClassificationsResponse{
				"origin1": {
					Origin: "origin1",
					From:   now,
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 50,
						Failure: 40,
					},
				},
				"origin2": {
					Origin: "origin2",
					From:   now,
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 30,
						Failure: 70,
					},
				},
				"origin4": {
					Origin: "origin4",
					From:   now,
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
		{
			name: "Only today's metrics are included when the timespan only includes today",
			from: time.Now(),
			to:   time.Now().AddDate(0, 0, 2),
			expected: map[types.PortalAppOrigin]OriginClassificationsResponse{
				"origin1": {
					Origin: "origin1",
					From:   now,
					To:     now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 50,
						Failure: 40,
					},
				},
				"origin2": {
					Origin: "origin2",
					From:   now,
					To:     now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 30,
						Failure: 70,
					},
				},
				"origin4": {
					Origin: "origin4",
					From:   now,
					To:     now.AddDate(0, 0, 3),
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
		{
			name: "Missing parameters' default values",
			expected: map[types.PortalAppOrigin]OriginClassificationsResponse{
				"origin1": {
					Origin: "origin1",
					From:   now.AddDate(0, 0, -30),
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 50,
						Failure: 40,
					},
				},
				"origin2": {
					Origin: "origin2",
					From:   now.AddDate(0, 0, -30),
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 30,
						Failure: 70,
					},
				},
				"origin4": {
					Origin: "origin4",
					From:   now.AddDate(0, 0, -30),
					To:     now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 500,
						Failure: 700,
					},
				},
			},
		},
		{
			name:        "Invalid timespan is rejected",
			from:        now.AddDate(0, 0, -1),
			to:          now.AddDate(0, 0, -2),
			expectedErr: fmt.Errorf("Invalid timespan"),
			expected:    map[types.PortalAppOrigin]OriginClassificationsResponse{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBackend := fakeBackend{
				todaysOriginUsage: todaysUsage,
			}

			relayMeter := NewRelayMeter(context.Background(), &fakeBackend, &fakeDriver{}, logger.New(), RelayMeterOptions{LoadInterval: 100 * time.Millisecond})
			time.Sleep(200 * time.Millisecond)
			rawGot, err := relayMeter.AllRelaysOrigin(context.Background(), tc.from, tc.to)
			if err != nil {
				if tc.expectedErr == nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), tc.expectedErr.Error()) {
					t.Fatalf("Expected error to contain: %q, got: %v", tc.expectedErr.Error(), err)
				}
			}

			// Need to convert it to map to be able to compare
			got := make(map[types.PortalAppOrigin]OriginClassificationsResponse, len(rawGot))

			for _, relResp := range rawGot {
				got[relResp.Origin] = relResp
			}

			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeBackend struct {
	usage              map[time.Time]map[types.PortalAppPublicKey]RelayCounts
	err                error
	todaysUsage        map[types.PortalAppPublicKey]RelayCounts
	todaysOriginUsage  map[types.PortalAppOrigin]RelayCounts
	todaysLatency      map[types.PortalAppPublicKey][]Latency
	userApps           map[types.UserID][]types.PortalAppPublicKey
	todaysMetricsCalls int
	todaysLatencyCalls int
	dailyMetricsCalls  int
	dailyMetricsFrom   time.Time
	dailyMetricsTo     time.Time

	portalApps map[types.PortalAppID]*types.PortalApp
}

func (f *fakeBackend) DailyUsage(from, to time.Time) (map[time.Time]map[types.PortalAppPublicKey]RelayCounts, error) {
	f.dailyMetricsCalls++
	f.dailyMetricsFrom = from
	f.dailyMetricsTo = to
	return f.usage, f.err
}

func (f *fakeBackend) TodaysUsage() (map[types.PortalAppPublicKey]RelayCounts, error) {
	f.todaysMetricsCalls++
	return f.todaysUsage, f.err
}

func (f *fakeBackend) TodaysLatency() (map[types.PortalAppPublicKey][]Latency, error) {
	f.todaysLatencyCalls++
	return f.todaysLatency, f.err
}

func (f *fakeBackend) TodaysOriginUsage() (map[types.PortalAppOrigin]RelayCounts, error) {
	return f.todaysOriginUsage, nil
}

func (f *fakeBackend) UserPortalAppPubKeys(ctx context.Context, user types.UserID) ([]types.PortalAppPublicKey, error) {
	return f.userApps[user], nil
}

func (f *fakeBackend) PortalApp(ctx context.Context, portalAppID types.PortalAppID) (*types.PortalApp, error) {
	return f.portalApps[portalAppID], f.err
}

func (f *fakeBackend) PortalApps(ctx context.Context) ([]*types.PortalApp, error) {
	var lbs []*types.PortalApp

	for _, lb := range f.portalApps {
		lbs = append(lbs, lb)
	}

	return lbs, f.err
}

func fakeDailyMetrics() map[time.Time]map[types.PortalAppPublicKey]RelayCounts {
	dayMetrics := map[types.PortalAppPublicKey]RelayCounts{
		"app1": {Success: 2, Failure: 3},
		"app2": {Success: 1, Failure: 5},
		"app4": {Success: 5, Failure: 7},
	}

	now, _ := time.Parse(dayFormat, time.Now().Format(dayFormat))
	metrics := make(map[time.Time]map[types.PortalAppPublicKey]RelayCounts)
	for i := 1; i < 7; i++ {
		metrics[now.AddDate(0, 0, -1*i)] = dayMetrics
	}
	return metrics
}

func fakeTodaysMetrics() map[types.PortalAppPublicKey]RelayCounts {
	return map[types.PortalAppPublicKey]RelayCounts{
		"app1": {Success: 50, Failure: 40},
		"app2": {Success: 30, Failure: 70},
		"app4": {Success: 500, Failure: 700},
	}
}

func fakeTodaysMetricsByOrigin() map[types.PortalAppOrigin]RelayCounts {
	return map[types.PortalAppOrigin]RelayCounts{
		"origin1": {Success: 50, Failure: 40},
		"origin2": {Success: 30, Failure: 70},
		"origin4": {Success: 500, Failure: 700},
	}
}

func fakeTodaysLatency() map[types.PortalAppPublicKey][]Latency {
	hourFormat := "2006-01-02 15:04:00Z"
	now, _ := time.Parse(hourFormat, time.Now().Format(hourFormat))

	latencyMetrics := []Latency{}

	for i := 0; i < 24; i++ {
		latencyMetrics = append(latencyMetrics, Latency{
			Time:    now.Add(-(time.Hour * time.Duration(24-i+1))),
			Latency: numbers.RoundFloat(0.11111+(float64(i)*0.01111), 5),
		})
	}

	return map[types.PortalAppPublicKey][]Latency{
		"app1": latencyMetrics,
		"app2": latencyMetrics,
		"app4": latencyMetrics,
	}
}

type fakeDriver struct{}

func (d *fakeDriver) WriteHTTPSourceRelayCounts(ctx context.Context, counts []HTTPSourceRelayCount) error {
	return nil
}

// sortPublicKeys sorts a slice of types.PortalAppPublicKey for comparison in tests
func sortPublicKeys(publicKeys []types.PortalAppPublicKey) []types.PortalAppPublicKey {
	if len(publicKeys) == 0 {
		return nil
	}

	keys := make([]string, len(publicKeys))
	for i, key := range publicKeys {
		keys[i] = string(key)
	}

	// Sort the slice
	sort.Strings(keys)

	// Convert back to a slice of types.PortalAppPublicKey
	sortedKeys := make([]types.PortalAppPublicKey, len(keys))
	for i, key := range keys {
		sortedKeys[i] = types.PortalAppPublicKey(key)
	}

	return sortedKeys
}
