package collector

import (
	"time"

	"github.com/pokt-foundation/relay-meter/api"
)

func mergeTimeRelayCountsMaps(dayMaps []map[time.Time]map[string]api.RelayCounts) map[time.Time]map[string]api.RelayCounts {
	rawMergedMap := make(map[time.Time][]map[string]api.RelayCounts)

	// first we separate the app maps by day
	for _, dayMap := range dayMaps {
		for day, appMap := range dayMap {
			rawMergedMap[day] = append(rawMergedMap[day], appMap)
		}
	}

	mergedMap := make(map[time.Time]map[string]api.RelayCounts)

	// then we merge the app maps of each day
	for day, appMapSlice := range rawMergedMap {
		mergedMap[day] = mergeRelayCountsMaps(appMapSlice)
	}

	return mergedMap
}

func mergeRelayCountsMaps(appMaps []map[string]api.RelayCounts) map[string]api.RelayCounts {
	mergedMap := make(map[string]api.RelayCounts)

	for _, appMap := range appMaps {
		for app, count := range appMap {
			if _, ok := mergedMap[app]; ok {
				mergedMap[app] = api.RelayCounts{
					Success: mergedMap[app].Success + count.Success,
					Failure: mergedMap[app].Failure + count.Failure,
				}
			} else {
				mergedMap[app] = count
			}
		}
	}

	return mergedMap
}

func mergeLatencyMaps(latencyMaps []map[string][]api.Latency) map[string][]api.Latency {
	mergedMap := make(map[string][]api.Latency)

	for _, latencyMap := range latencyMaps {
		for app, latency := range latencyMap {
			mergedMap[app] = append(mergedMap[app], latency...)
		}
	}

	return mergedMap
}
