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
)

const (
	DAY_LAYOUT       = "2006-01-02"
	TABLE_DAILY_SUMS = "daily_app_sums"
)

// Will be implemented by Postgres DB interface
type Reporter interface {
	// DailyUsage returns saved daily metrics for the specified time period, with each day being an entry in the results map
	DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error)
}

// Will be implemented by Postgres DB interface
type Writer interface {
	// WriteUsage writes the specified relay counts to the underlying storage.
	//	It also writes the lastCounted to the underlying storage to keep track of most recent run of the meter.
	WriteUsage(counts map[string]int64, lastCounted time.Time) error
	// TODO: function to read entries, needed by the rollover
	// TODO: rollover of entries
	WriteDailyUsage(counts map[time.Time]map[string]int64) error
	// WriteTodaysUsage writes todays relay counts to the underlying storage.
	WriteTodaysUsage(counts map[string]int64) error
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

func (p *pgClient) DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error) {
	ctx := context.Background()
	// TODO: delegate dealing with the timestamps to the sql query: looks like there is a bug in QueryContext in dealing with parameters
	q := fmt.Sprintf("SELECT (time, application, count) FROM daily_app_sums as d WHERE d.time >= '%s' and d.time <= '%s'",
		from.Format(DAY_LAYOUT),
		to.Format(DAY_LAYOUT),
	)
	rows, err := p.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	const timeFormat = "2006-01-02 15:04:00+00"
	dailyUsage := make(map[time.Time]map[string]int64)
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
		if len(items) != 3 {
			return nil, fmt.Errorf("Invalid format in query output: %s", r)
		}

		ts, err := time.Parse(timeFormat, items[0])
		if err != nil {
			return nil, fmt.Errorf("Invalid time format: %s in query result line: %s, error: %v", items[0], r, err)
		}
		count, err := strconv.ParseInt(items[2], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		app := items[1]
		if app == "" {
			return nil, fmt.Errorf("Empty application public key, in query result line: %s", r)
		}

		if dailyUsage[ts] == nil {
			dailyUsage[ts] = make(map[string]int64)
		}
		dailyUsage[ts][app] = count
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

func (p *pgClient) WriteDailyUsage(counts map[time.Time]map[string]int64) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// TODO: bulk insert
	for day, appCounts := range counts {
		for app, count := range appCounts {
			_, execErr := tx.ExecContext(ctx, "INSERT INTO daily_app_sums(application, count, time) VALUES($1, $2, $3);", app, count, day)
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

func (p *pgClient) WriteUsage(counts map[string]int64, lastCounted time.Time) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// TODO: bulk insert
	for app, count := range counts {
		_, execErr := tx.ExecContext(ctx, "INSERT INTO relay_counts(application, count, time) VALUES($1, $2, $3);", app, count, lastCounted)
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
	first, err := p.parseDate(firstStr)
	if err != nil {
		return first, last, err
	}
	last, err = p.parseDate(lastStr)
	return first, last, err
}

// WriteTodaysUsage writes the app metrics for today so far to the underlying PG table.
//	All the entries in the table holding todays metrics are deleted first.
func (p *pgClient) WriteTodaysUsage(counts map[string]int64) error {
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
		_, execErr := tx.ExecContext(ctx, "INSERT INTO todays_app_sums(application, count) VALUES($1, $2);", app, count)
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

func (p *pgClient) parseDate(source string) (time.Time, error) {
	// Postgres queries date output format: 2022-05-31T00:00:00Z
	const layout = "2006-01-02T15:04:00Z"
	return time.Parse(layout, source)
}
