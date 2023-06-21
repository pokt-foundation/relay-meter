package collector

import (
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
)

func mergeTimeRelayCountsMaps(dayMaps []map[time.Time]map[types.PortalAppID]api.RelayCounts) map[time.Time]map[types.PortalAppID]api.RelayCounts {
	rawMergedMap := make(map[time.Time][]map[types.PortalAppID]api.RelayCounts)

	// first we separate the portalAppID maps by day
	for _, dayMap := range dayMaps {
		for day, appMap := range dayMap {
			rawMergedMap[day] = append(rawMergedMap[day], appMap)
		}
	}

	mergedMap := make(map[time.Time]map[types.PortalAppID]api.RelayCounts)

	// then we merge the portalAppID maps of each day
	for day, appMapSlice := range rawMergedMap {
		mergedMap[day] = mergeRelayCountsMaps(appMapSlice)
	}

	return mergedMap
}

func mergeRelayCountsMaps(appMaps []map[types.PortalAppID]api.RelayCounts) map[types.PortalAppID]api.RelayCounts {
	mergedMap := make(map[types.PortalAppID]api.RelayCounts)

	for _, appMap := range appMaps {
		for portalAppID, count := range appMap {
			if _, ok := mergedMap[portalAppID]; ok {
				mergedMap[portalAppID] = api.RelayCounts{
					Success: mergedMap[portalAppID].Success + count.Success,
					Failure: mergedMap[portalAppID].Failure + count.Failure,
				}
			} else {
				mergedMap[portalAppID] = count
			}
		}
	}

	return mergedMap
}

func mergeOriginRelayCountsMaps(appMaps []map[string]api.RelayCounts) map[string]api.RelayCounts {
	mergedMap := make(map[string]api.RelayCounts)

	for _, appMap := range appMaps {
		for origin, count := range appMap {
			if _, ok := mergedMap[origin]; ok {
				mergedMap[origin] = api.RelayCounts{
					Success: mergedMap[origin].Success + count.Success,
					Failure: mergedMap[origin].Failure + count.Failure,
				}
			} else {
				mergedMap[origin] = count
			}
		}
	}

	return mergedMap
}

func mergeLatencyMaps(latencyMaps []map[types.PortalAppID][]api.Latency) map[types.PortalAppID][]api.Latency {
	mergedMap := make(map[types.PortalAppID][]api.Latency)

	for _, latencyMap := range latencyMaps {
		for portalAppID, latency := range latencyMap {
			mergedMap[portalAppID] = append(mergedMap[portalAppID], latency...)
		}
	}

	return mergedMap
}
