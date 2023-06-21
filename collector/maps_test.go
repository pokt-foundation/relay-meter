package collector

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
)

func TestMergeApps(t *testing.T) {
	fakeDay1 := time.Date(2022, time.July, 20, 0, 0, 0, 0, &time.Location{})
	fakeDay2 := fakeDay1.AddDate(0, 0, 1)
	fakeDay3 := fakeDay1.AddDate(0, 0, 2)

	source1 := map[time.Time]map[types.PortalAppID]api.RelayCounts{
		fakeDay2: {
			"test_956d67d3ea93cbfe18a": {
				Success: 4,
				Failure: 4,
			},
			"test_6b2faf2e3b061651297": {
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"test_956d67d3ea93cbfe18a": {
				Success: 3,
				Failure: 3,
			},
			"test_6b2faf2e3b061651297": {
				Success: 3,
				Failure: 3,
			},
		},
	}

	source2 := map[time.Time]map[types.PortalAppID]api.RelayCounts{
		fakeDay3: {
			"test_adf93ee1bbb6d617c98": {
				Success: 3,
				Failure: 3,
			},
		},
		fakeDay2: {
			"test_adf93ee1bbb6d617c98": {
				Success: 4,
				Failure: 4,
			},
			"test_956d67d3ea93cbfe18a": {
				Success: 4,
				Failure: 4,
			},
			"test_4711afda9f2ec4d3165": {
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"test_adf93ee1bbb6d617c98": {
				Success: 3,
				Failure: 3,
			},
			"test_4711afda9f2ec4d3165": {
				Success: 3,
				Failure: 3,
			},
		},
	}

	expectedSource := map[time.Time]map[types.PortalAppID]api.RelayCounts{
		fakeDay3: {
			"test_adf93ee1bbb6d617c98": {
				Success: 3,
				Failure: 3,
			},
		},
		fakeDay2: {
			"test_956d67d3ea93cbfe18a": {
				Success: 8,
				Failure: 8,
			},
			"test_6b2faf2e3b061651297": {
				Success: 4,
				Failure: 4,
			},
			"test_adf93ee1bbb6d617c98": {
				Success: 4,
				Failure: 4,
			},
			"test_4711afda9f2ec4d3165": {
				Success: 4,
				Failure: 4,
			},
		},
		fakeDay1: {
			"test_956d67d3ea93cbfe18a": {
				Success: 3,
				Failure: 3,
			},
			"test_6b2faf2e3b061651297": {
				Success: 3,
				Failure: 3,
			},
			"test_adf93ee1bbb6d617c98": {
				Success: 3,
				Failure: 3,
			},
			"test_4711afda9f2ec4d3165": {
				Success: 3,
				Failure: 3,
			},
		},
	}

	source := mergeTimeRelayCountsMaps([]map[time.Time]map[types.PortalAppID]api.RelayCounts{source1, source2})

	if !cmp.Equal(source, expectedSource) {
		t.Errorf("Wrong object received, got=%s", cmp.Diff(expectedSource, source))
	}
}

func TestMergeLatencies(t *testing.T) {
	fakeDay1 := time.Date(2022, time.July, 20, 0, 0, 0, 0, &time.Location{})
	fakeDay2 := fakeDay1.AddDate(0, 0, 1)

	source1 := map[types.PortalAppID][]api.Latency{
		"test_956d67d3ea93cbfe18a": {
			{
				Time:    fakeDay1,
				Latency: 21.07,
			},
		},
		"test_6b2faf2e3b061651297": {
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	source2 := map[types.PortalAppID][]api.Latency{
		"test_956d67d3ea93cbfe18a": {
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"test_4711afda9f2ec4d3165": {
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	expectedSource := map[types.PortalAppID][]api.Latency{
		"test_956d67d3ea93cbfe18a": {
			{
				Time:    fakeDay1,
				Latency: 21.07,
			},
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"test_4711afda9f2ec4d3165": {
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
		"test_6b2faf2e3b061651297": {
			{
				Time:    fakeDay2,
				Latency: 21.07,
			},
		},
	}

	source := mergeLatencyMaps([]map[types.PortalAppID][]api.Latency{source1, source2})

	if !cmp.Equal(source, expectedSource) {
		t.Errorf("Wrong object received, got=%s", cmp.Diff(expectedSource, source))
	}
}
