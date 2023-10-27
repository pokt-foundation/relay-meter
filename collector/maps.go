package collector

import (
	"time"

	"github.com/pokt-foundation/portal-http-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
)

func mergeTimeRelayCountsMaps(dayMaps []map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts) map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts {
	rawMergedMap := make(map[time.Time][]map[types.PortalAppPublicKey]api.RelayCounts)

	// first we separate the app maps by day
	for _, dayMap := range dayMaps {
		for day, appMap := range dayMap {
			rawMergedMap[day] = append(rawMergedMap[day], appMap)
		}
	}

	mergedMap := make(map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts)

	// then we merge the app maps of each day
	for day, appMapSlice := range rawMergedMap {
		mergedMap[day] = mergeRelayCountsMaps(appMapSlice)
	}

	return mergedMap
}

func mergeRelayCountsMaps(appMaps []map[types.PortalAppPublicKey]api.RelayCounts) map[types.PortalAppPublicKey]api.RelayCounts {
	mergedMap := make(map[types.PortalAppPublicKey]api.RelayCounts)

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

func mergeRelayCountsMapsByOrigin(appMaps []map[types.PortalAppOrigin]api.RelayCounts) map[types.PortalAppOrigin]api.RelayCounts {
	mergedMap := make(map[types.PortalAppOrigin]api.RelayCounts)

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

func mergeLatencyMaps(latencyMaps []map[types.PortalAppPublicKey][]api.Latency) map[types.PortalAppPublicKey][]api.Latency {
	mergedMap := make(map[types.PortalAppPublicKey][]api.Latency)

	for _, latencyMap := range latencyMaps {
		for app, latency := range latencyMap {
			mergedMap[app] = append(mergedMap[app], latency...)
		}
	}

	return mergedMap
}
