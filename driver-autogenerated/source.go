package postgresdriver

import (
	"context"
	"time"

	"github.com/pokt-foundation/portal-http-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
)

func (d *PostgresDriver) DailyCounts(from, to time.Time) (map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts, error) {
	relayCountsByString := make(map[string]map[types.PortalAppPublicKey]api.RelayCounts)

	// create map for each date between from and to
	date := from
	for !date.After(to) {
		relayCountsByString[date.Format("2006-01-02")] = make(map[types.PortalAppPublicKey]api.RelayCounts)
		date = date.AddDate(0, 0, 1)
	}

	counts, err := d.ReadHTTPSourceRelayCounts(context.Background(), from, to)
	if err != nil {
		return map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts{}, err
	}

	for _, count := range counts {
		// get the date string for this count
		dateStr := count.Day.Format("2006-01-02")

		// update the relayCounts map for the given date and appPublicKey
		if countsMap, ok := relayCountsByString[dateStr]; ok {
			countsMap[count.AppPublicKey] = api.RelayCounts{
				Success: count.Success,
				Failure: count.Error,
			}
		}
	}

	relayCounts := make(map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts)

	// convert to map by time for consistency with the interface
	// we need to use the same format as the input so all sources have equal dates when needed
	date = from
	for !date.After(to) {
		relayCounts[date] = relayCountsByString[date.Format("2006-01-02")]
		date = date.AddDate(0, 0, 1)
	}

	return relayCounts, nil
}

func (d *PostgresDriver) TodaysCounts() (map[types.PortalAppPublicKey]api.RelayCounts, error) {
	now := time.Now()

	counts, err := d.ReadHTTPSourceRelayCounts(context.Background(), now, now)
	if err != nil {
		return map[types.PortalAppPublicKey]api.RelayCounts{}, nil
	}

	relayCounts := make(map[types.PortalAppPublicKey]api.RelayCounts)

	for _, count := range counts {
		relayCounts[count.AppPublicKey] = api.RelayCounts{
			Success: count.Success,
			Failure: count.Error,
		}
	}

	return relayCounts, nil
}

func (d *PostgresDriver) TodaysCountsPerOrigin() (map[types.PortalAppOrigin]api.RelayCounts, error) {
	return map[types.PortalAppOrigin]api.RelayCounts{}, nil
}

func (d *PostgresDriver) TodaysLatency() (map[types.PortalAppPublicKey][]api.Latency, error) {
	return map[types.PortalAppPublicKey][]api.Latency{}, nil
}

func (d *PostgresDriver) Name() string {
	return "http"
}
