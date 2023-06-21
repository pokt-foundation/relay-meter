package postgresdriver

import (
	"context"
	"time"

	"github.com/pokt-foundation/relay-meter/api"
)

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, &time.Location{})
}

func (d *PostgresDriver) WriteHTTPSourceRelayCount(ctx context.Context, count api.HTTPSourceRelayCount) error {
	return d.InsertHTTPSourceRelayCount(ctx, InsertHTTPSourceRelayCountParams{
		PortalAppID: count.PortalAppID,
		Day:         truncateToDay(count.Day),
		Success:     count.Success,
		Error:       count.Error,
	})
}

func (d *PostgresDriver) WriteHTTPSourceRelayCounts(ctx context.Context, counts []api.HTTPSourceRelayCount) error {
	var (
		portalAppIDs []string
		days         []time.Time
		successes    []int64
		errors       []int64
	)

	for _, count := range counts {
		portalAppIDs = append(portalAppIDs, string(count.PortalAppID))
		days = append(days, truncateToDay(count.Day))
		successes = append(successes, count.Success)
		errors = append(errors, count.Error)
	}

	return d.InsertHTTPSourceRelayCounts(ctx, InsertHTTPSourceRelayCountsParams{
		Column1: portalAppIDs,
		Column2: days,
		Column3: successes,
		Column4: errors,
	})
}

func (d *PostgresDriver) ReadHTTPSourceRelayCounts(ctx context.Context, from, to time.Time) ([]api.HTTPSourceRelayCount, error) {
	dbCounts, err := d.SelectHTTPSourceRelayCounts(ctx, SelectHTTPSourceRelayCountsParams{
		Day:   truncateToDay(from),
		Day_2: truncateToDay(to),
	})
	if err != nil {
		return nil, err
	}

	var counts []api.HTTPSourceRelayCount

	for _, dbCount := range dbCounts {
		counts = append(counts, api.HTTPSourceRelayCount{
			PortalAppID: dbCount.PortalAppID,
			Day:         dbCount.Day,
			Success:     dbCount.Success,
			Error:       dbCount.Error,
		})
	}

	return counts, nil
}
