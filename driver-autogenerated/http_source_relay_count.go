package postgresdriver

import (
	"context"
	"time"
)

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, &time.Location{})
}

func (d *PostgresDriver) WriteHTTPSourceRelayCount(ctx context.Context, count HttpSourceRelayCount) error {
	return d.InsertHTTPSourceRelayCount(ctx, InsertHTTPSourceRelayCountParams{
		AppPublicKey: count.AppPublicKey,
		Day:          truncateToDay(count.Day),
		Success:      count.Success,
		Error:        count.Error,
	})
}

func (d *PostgresDriver) WriteHTTPSourceRelayCounts(ctx context.Context, counts []HttpSourceRelayCount) error {
	var (
		appPublicKeys []string
		days          []time.Time
		successes     []int64
		errors        []int64
	)

	for _, count := range counts {
		appPublicKeys = append(appPublicKeys, count.AppPublicKey)
		days = append(days, truncateToDay(count.Day))
		successes = append(successes, count.Success)
		errors = append(errors, count.Error)
	}

	return d.InsertHTTPSourceRelayCounts(ctx, InsertHTTPSourceRelayCountsParams{
		Column1: appPublicKeys,
		Column2: days,
		Column3: successes,
		Column4: errors,
	})
}

func (d *PostgresDriver) ReadHTTPSourceRelayCounts(ctx context.Context, from, to time.Time) ([]HttpSourceRelayCount, error) {
	dbCounts, err := d.SelectHTTPSourceRelayCounts(ctx, SelectHTTPSourceRelayCountsParams{
		Day:   truncateToDay(from),
		Day_2: truncateToDay(to),
	})
	if err != nil {
		return nil, err
	}

	var counts []HttpSourceRelayCount

	for _, dbCount := range dbCounts {
		counts = append(counts, HttpSourceRelayCount{
			AppPublicKey: dbCount.AppPublicKey,
			Day:          dbCount.Day,
			Success:      dbCount.Success,
			Error:        dbCount.Error,
		})
	}

	return counts, nil
}
