package postgresdriver

import (
	"context"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
)

func (d *PostgresDriver) DailyCounts(from time.Time, to time.Time) (map[time.Time]map[types.PortalAppID]api.RelayCounts, error) {
	relayCountsByString := make(map[string]map[types.PortalAppID]api.RelayCounts)

	// create map for each date between from and to
	date := from
	for !date.After(to) {
		relayCountsByString[date.Format("2006-01-02")] = make(map[types.PortalAppID]api.RelayCounts)
		date = date.AddDate(0, 0, 1)
	}

	counts, err := d.ReadHTTPSourceRelayCounts(context.Background(), from, to)
	if err != nil {
		return map[time.Time]map[types.PortalAppID]api.RelayCounts{}, err
	}

	for _, count := range counts {
		// get the date string for this count
		dateStr := count.Day.Format("2006-01-02")

		// update the relayCounts map for the given date and appPublicKey
		if countsMap, ok := relayCountsByString[dateStr]; ok {
			countsMap[count.PortalAppID] = api.RelayCounts{
				Success: count.Success,
				Failure: count.Error,
			}
		}
	}

	relayCounts := make(map[time.Time]map[types.PortalAppID]api.RelayCounts)

	// convert to map by time for consistency with the interface
	// we need to use the same format as the input so all sources have equal dates when needed
	date = from
	for !date.After(to) {
		relayCounts[date] = relayCountsByString[date.Format("2006-01-02")]
		date = date.AddDate(0, 0, 1)
	}

	return relayCounts, nil
}

func (d *PostgresDriver) TodaysCounts() (map[types.PortalAppID]api.RelayCounts, error) {
	now := time.Now()

	counts, err := d.ReadHTTPSourceRelayCounts(context.Background(), now, now)
	if err != nil {
		return map[types.PortalAppID]api.RelayCounts{}, nil
	}

	relayCounts := make(map[types.PortalAppID]api.RelayCounts)

	for _, count := range counts {
		relayCounts[count.PortalAppID] = api.RelayCounts{
			Success: count.Success,
			Failure: count.Error,
		}
	}

	return relayCounts, nil
}

func (d *PostgresDriver) TodaysCountsPerOrigin() (map[string]api.RelayCounts, error) {
	return map[string]api.RelayCounts{}, nil
}

func (d *PostgresDriver) TodaysLatency() (map[types.PortalAppID][]api.Latency, error) {
	return map[types.PortalAppID][]api.Latency{}, nil
}

func (d *PostgresDriver) Name() string {
	return "http"
}
