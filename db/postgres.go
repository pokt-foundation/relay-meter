package db

// TODO: do we need a more secure way of passing the passwords?

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"database/sql"

	_ "github.com/lib/pq"

	"github.com/adshmh/meter/api"
)

// TODO: db package needs some form of unit testing
const (
	DAY_LAYOUT       = "2006-01-02"
	TABLE_DAILY_SUMS = "daily_app_sums"
)

// Will be implemented by Postgres DB interface
type Reporter interface {
	// DailyUsage returns saved daily metrics for the specified time period, with each day being an entry in the results map
	DailyUsage(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error)
	// TodaysUsage returns the metrics for today so far
	TodaysUsage() (map[string]api.RelayCounts, error)
}

// Will be implemented by Postgres DB interface
type Writer interface {
	// TODO: rollover of entries
	WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts) error
	// WriteTodaysMetrics writes todays relay counts and latencies to the underlying storage.
	WriteTodaysMetrics(counts map[string]api.RelayCounts, latencies map[string][]api.Latency) error
	// Returns oldest and most recent timestamps for stored metrics
	ExistingMetricsTimespan() (time.Time, time.Time, error)
}

type PostgresOptions struct {
	Host     string
	User     string
	Password string
	DB       string
}

type PostgresClient interface {
	Reporter
	Writer
}

func NewPostgresClient(options PostgresOptions) (PostgresClient, error) {
	// TODO: add '?sslmode=verify-full' to connection string?
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", options.User, options.Password, options.Host, options.DB)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	return &pgClient{DB: db}, nil
}

// type pgReporter
type pgClient struct {
	*sql.DB
}

func (p *pgClient) DailyUsage(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error) {
	ctx := context.Background()
	// TODO: delegate dealing with the timestamps to the sql query: looks like there is a bug in QueryContext in dealing with parameters
	q := fmt.Sprintf("SELECT (time, application, count_success, count_failure) FROM daily_app_sums as d WHERE d.time >= '%s' and d.time <= '%s'",
		from.Format(DAY_LAYOUT),
		to.Format(DAY_LAYOUT),
	)
	rows, err := p.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	const timeFormat = "2006-01-02 15:04:00+00"
	dailyUsage := make(map[time.Time]map[string]api.RelayCounts)
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}

		// Example of query output (app public key has been modified)
		// ("2022-06-25 00:00:00+00",33d4474f0a60b362103b1867c7edac323e39f416e7458f436623b9d96eb31k19,18931)
		r = strings.ReplaceAll(r, "\"", "")
		r = strings.TrimPrefix(r, "(")
		r = strings.TrimSuffix(r, ")")
		items := strings.Split(r, ",")
		if len(items) != 4 {
			return nil, fmt.Errorf("Invalid format in query output: %s", r)
		}

		ts, err := time.Parse(timeFormat, items[0])
		if err != nil {
			return nil, fmt.Errorf("Invalid time format: %s in query result line: %s, error: %v", items[0], r, err)
		}
		count_success, err := strconv.ParseInt(items[2], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		count_failure, err := strconv.ParseInt(items[3], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[3], r, err)
		}
		app := items[1]
		if app == "" {
			return nil, fmt.Errorf("Empty application public key, in query result line: %s", r)
		}

		if dailyUsage[ts] == nil {
			dailyUsage[ts] = make(map[string]api.RelayCounts)
		}
		dailyUsage[ts][app] = api.RelayCounts{Success: count_success, Failure: count_failure}
	}
	// TODO: verify this is needed
	if rerr := rows.Close(); rerr != nil {
		return nil, rerr
	}
	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return dailyUsage, nil
}

func (p *pgClient) WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// TODO: bulk insert
	for day, appCounts := range counts {
		for app, counts := range appCounts {
			_, execErr := tx.ExecContext(ctx,
				"INSERT INTO daily_app_sums(application, count_success, count_failure, time) VALUES($1, $2, $3, $4);",
				app, counts.Success, counts.Failure, day)
			if execErr != nil {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					fmt.Printf("update failed: %v, unable to rollback: %v\n", execErr, rollbackErr)
					return execErr
				}
				fmt.Printf("update failed: %v", execErr)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (p *pgClient) ExistingMetricsTimespan() (time.Time, time.Time, error) {
	ctx := context.Background()
	row := p.DB.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*), COALESCE(min(time), '2003-01-02 03:04' ), COALESCE(max(time), '2003-01-02 03:04') FROM %s", TABLE_DAILY_SUMS))
	var countStr, firstStr, lastStr string
	var first, last time.Time
	if err := row.Scan(&countStr, &firstStr, &lastStr); err != nil {
		return first, last, err
	}

	if countStr == "0" {
		return time.Time{}, time.Time{}, nil
	}
	first, err := parseDate(firstStr)
	if err != nil {
		return first, last, err
	}
	last, err = parseDate(lastStr)
	return first, last, err
}

func (p *pgClient) WriteTodaysMetrics(counts map[string]api.RelayCounts, latencies map[string][]api.Latency) error {
	err := p.writeTodaysUsage(counts)
	if err != nil {
		return fmt.Errorf("error writing usage: %s", err.Error())
	}

	err = p.writeTodaysLatency(latencies)
	if err != nil {
		return fmt.Errorf("error writing latency: %s", err.Error())
	}

	return nil
}

// WriteTodaysUsage writes the app metrics for today so far to the underlying PG table.
//	All the entries in the table holding todays metrics are deleted first.
func (p *pgClient) writeTodaysUsage(counts map[string]api.RelayCounts) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// todays_sums table gets rebuilt every time
	_, deleteErr := tx.ExecContext(ctx, "DELETE FROM todays_app_sums")
	if deleteErr != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			fmt.Printf("delete failed: %v, unable to rollback: %v\n", deleteErr, rollbackErr)
			return deleteErr
		}
	}

	// TODO: bulk insert
	for app, count := range counts {
		_, execErr := tx.ExecContext(ctx,
			"INSERT INTO todays_app_sums(application, count_success, count_failure) VALUES($1, $2, $3);",
			app, count.Success, count.Failure)
		if execErr != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				fmt.Printf("update failed: %v, unable to rollback: %v\n", execErr, rollbackErr)
				return execErr
			}
			fmt.Printf("update failed: %v", execErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// WriteTodaysUsage writes the app metrics for today so far to the underlying PG table.
//	All the entries in the table holding todays metrics are deleted first.
func (p *pgClient) writeTodaysLatency(latencies map[string][]api.Latency) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// todays_app_latencies table gets rebuilt every time
	_, deleteErr := tx.ExecContext(ctx, "DELETE FROM todays_app_latencies")
	if deleteErr != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			fmt.Printf("delete failed: %v, unable to rollback: %v\n", deleteErr, rollbackErr)
			return deleteErr
		}
	}

	// TODO: bulk insert
	for app, appLatency := range latencies {
		for _, appLatency := range appLatency {
			_, execErr := tx.ExecContext(ctx,
				"INSERT INTO todays_app_latencies(application, time, latency) VALUES($1, $2, $3);",
				app, appLatency.Time, appLatency.Latency)

			if execErr != nil {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					fmt.Printf("update failed: %v, unable to rollback: %v\n", execErr, rollbackErr)
					return execErr
				}
				fmt.Printf("update failed: %v", execErr)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// TodaysUsage returns the current day's metrics so far.
func (pg *pgClient) TodaysUsage() (map[string]api.RelayCounts, error) {
	// TODO: factor-out the SQL statements
	ctx := context.Background()
	rows, err := pg.DB.QueryContext(ctx, "SELECT (application, count_success, count_failure) FROM todays_app_sums")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todaysUsage := make(map[string]api.RelayCounts)
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}

		// Example of query output:
		// (46d4474f0a60f062103b1867c7edac323e58f416e7458f436623b9d96eb44b37,18931)
		r = strings.ReplaceAll(r, "\"", "")
		r = strings.TrimPrefix(r, "(")
		r = strings.TrimSuffix(r, ")")
		items := strings.Split(r, ",")
		if len(items) != 3 {
			return nil, fmt.Errorf("Invalid format in query output: %s", r)
		}

		count_success, err := strconv.ParseInt(items[1], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[1], r, err)
		}
		count_failure, err := strconv.ParseInt(items[2], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		app := items[0]
		if app == "" {
			return nil, fmt.Errorf("Empty application public key, in query result line: %s", r)
		}

		todaysUsage[app] = api.RelayCounts{Success: count_success, Failure: count_failure}
	}
	// TODO: verify this is needed
	if rerr := rows.Close(); rerr != nil {
		return nil, rerr
	}
	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return todaysUsage, nil
}

func parseDate(source string) (time.Time, error) {
	// Postgres queries date output format: 2022-05-31T00:00:00Z
	const layout = "2006-01-02T15:04:00Z"
	return time.Parse(layout, source)
}
