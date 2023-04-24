package postgresdriver

import (
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pokt-foundation/relay-meter/api"
)

func (ts *PGDriverTestSuite) TestPostgresDriver_DailyCounts() {
	tests := []struct {
		name     string
		from     time.Time
		to       time.Time
		expected map[time.Time]map[string]api.RelayCounts
		err      error
	}{
		{
			name: "Success",
			from: ts.from,
			to:   ts.to,
			expected: map[time.Time]map[string]api.RelayCounts{
				ts.to: {
					"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
						Success: 4,
						Failure: 4,
					},
					"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
						Success: 4,
						Failure: 4,
					},
				},
				ts.from: {
					"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
						Success: 3,
						Failure: 3,
					},
					"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
						Success: 3,
						Failure: 3,
					},
				},
			},
			err: nil,
		},
	}
	for _, tt := range tests {
		counts, err := ts.driver.DailyCounts(tt.from, tt.to)
		ts.Equal(err, tt.err)

		if !cmp.Equal(counts, tt.expected) {
			ts.T().Errorf("Wrong object received, got=%s", cmp.Diff(tt.expected, counts))
		}
	}
}

func (ts *PGDriverTestSuite) TestPostgresDriver_TodaysCounts() {
	tests := []struct {
		name     string
		expected map[string]api.RelayCounts
		err      error
	}{
		{
			name: "Success",
			expected: map[string]api.RelayCounts{
				"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
					Success: 5,
					Failure: 5,
				},
				"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
					Success: 5,
					Failure: 5,
				},
			},
			err: nil,
		},
	}
	for _, tt := range tests {
		counts, err := ts.driver.TodaysCounts()
		ts.Equal(err, tt.err)

		if !cmp.Equal(counts, tt.expected) {
			ts.T().Errorf("Wrong object received, got=%s", cmp.Diff(tt.expected, counts))
		}
	}
}

func (ts *PGDriverTestSuite) TestPostgresDriver_TodaysCountsPerOrigin() {
	tests := []struct {
		name     string
		expected map[string]api.RelayCounts
		err      error
	}{
		{
			name:     "Success",
			expected: map[string]api.RelayCounts{},
			err:      nil,
		},
	}
	for _, tt := range tests {
		counts, err := ts.driver.TodaysCountsPerOrigin()
		ts.Equal(err, tt.err)

		if !cmp.Equal(counts, tt.expected) {
			ts.T().Errorf("Wrong object received, got=%s", cmp.Diff(tt.expected, counts))
		}
	}
}

func (ts *PGDriverTestSuite) TestPostgresDriver_TodaysLatency() {
	tests := []struct {
		name     string
		expected map[string][]api.Latency
		err      error
	}{
		{
			name:     "Success",
			expected: map[string][]api.Latency{},
			err:      nil,
		},
	}
	for _, tt := range tests {
		counts, err := ts.driver.TodaysLatency()
		ts.Equal(err, tt.err)

		if !cmp.Equal(counts, tt.expected) {
			ts.T().Errorf("Wrong object received, got=%s", cmp.Diff(tt.expected, counts))
		}
	}
}

func (ts *PGDriverTestSuite) TestPostgresDriver_Name() {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "Success",
			expected: "http",
		},
	}
	for _, tt := range tests {
		ts.Equal(ts.driver.Name(), tt.expected)
	}
}
