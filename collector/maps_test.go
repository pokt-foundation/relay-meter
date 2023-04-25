package collector

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pokt-foundation/relay-meter/api"
)

func TestMergeApps(t *testing.T) {
	fakeDay1 := time.Date(2022, time.July, 20, 0, 0, 0, 0, &time.Location{})
	fakeDay2 := fakeDay1.AddDate(0, 0, 1)
	fakeDay3 := fakeDay1.AddDate(0, 0, 2)

	source1 := map[time.Time]map[string]api.RelayCounts{
		fakeDay2: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
		},
	}

	source2 := map[time.Time]map[string]api.RelayCounts{
		fakeDay3: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
		},
		fakeDay2: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
		},
	}

	expectedSource := map[time.Time]map[string]api.RelayCounts{
		fakeDay3: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
		},
		fakeDay2: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
				Success: 8,
				Failure: 8,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a7": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
			"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
				Success: 3,
				Failure: 3,
			},
		},
	}

	source := mergeTimeRelayCountsMaps([]map[time.Time]map[string]api.RelayCounts{source1, source2})

	if !cmp.Equal(source, expectedSource) {
		t.Errorf("Wrong object received, got=%s", cmp.Diff(expectedSource, source))
	}
}

func TestMergeLatencies(t *testing.T) {
	fakeDay1 := time.Date(2022, time.July, 20, 0, 0, 0, 0, &time.Location{})
	fakeDay2 := fakeDay1.AddDate(0, 0, 1)

	source1 := map[string][]api.Latency{
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
			{
				Time:    fakeDay1,
				Latency: 21.07,
			},
		},
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	source2 := map[string][]api.Latency{
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	expectedSource := map[string][]api.Latency{
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8": { // pragma: allowlist secret
			{
				Time:    fakeDay1,
				Latency: 21.07,
			},
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a6": { // pragma: allowlist secret
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9": { // pragma: allowlist secret
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	source := mergeLatencyMaps([]map[string][]api.Latency{source1, source2})

	if !cmp.Equal(source, expectedSource) {
		t.Errorf("Wrong object received, got=%s", cmp.Diff(expectedSource, source))
	}
}
