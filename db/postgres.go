package db

// TODO: do we need a more secure way of passing the passwords?

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	pgx "github.com/jackc/pgx/v5"
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/utils-go/numbers"

	// PQ import is required
	_ "github.com/lib/pq"
)

// TODO: db package needs some form of unit testing
const (
	pgTimeFormat   = "2006-01-02 15:04:00+00"
	dayLayout      = "2006-01-02"
	tableDailySums = "daily_app_sums"
)

var ()

// Will be implemented by Postgres DB interface
type Reporter interface {
	// DailyUsage returns saved daily metrics for the specified time period, with each day being an entry in the results map
	DailyUsage(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error)
	// TodaysUsage returns the metrics for today so far
	TodaysUsage() (map[string]api.RelayCounts, error)
	TodaysOriginUsage() (map[string]api.RelayCounts, error)
	TodaysLatency() (map[string][]api.Latency, error)
}

// Will be implemented by Postgres DB interface
type Writer interface {
	// TODO: rollover of entries
	WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error
	// WriteTodaysUsage writes todays relay counts to the underlying storage.
	WriteTodaysUsage(ctx context.Context, tx pgx.Tx, counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error
	WriteTodaysMetrics(counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts, latencies map[string][]api.Latency) error
	// Returns oldest and most recent timestamps for stored metrics
	ExistingMetricsTimespan() (time.Time, time.Time, error)
}

type PostgresOptions struct {
	Host                      string
	User                      string
	Password                  string
	DB                        string
	UsePrivate, EnableWriting bool
}

type CloudSQLConfig struct {
	DBUser                 string
	DBPassword             string
	DBName                 string
	InstanceConnectionName string
	PublicIP               string
	PrivateIP              string
}

type PostgresClient interface {
	Reporter
	Writer
}

// DO NOT use as a direct path to the db
//
// use NewPostgresClientFromDBInstance right after
func NewDBConnection(options PostgresOptions) (*pgx.Conn, error) {
	connectionDetails := ""
	ctx := context.Background()

	// Used for local testing
	if !options.UsePrivate {
		connectionDetails = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", options.User, options.Password, options.Host, options.DB)
		db, err := pgx.Connect(ctx, connectionDetails)
		if err != nil {
			return nil, err
		}

		return db, nil
	}

	var opts []cloudsqlconn.Option

	opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPrivateIP()))

	connectionDetails = fmt.Sprintf("user=%s dbname=%s", options.User, options.DB)
	config, err := pgx.ParseConfig(connectionDetails)
	if err != nil {
		return nil, err
	}

	dialer, err := cloudsqlconn.NewDialer(ctx, opts...)
	if err != nil {
		return nil, err
	}

	config.DialFunc = func(ctx context.Context, network, instance string) (net.Conn, error) {
		return dialer.Dial(ctx, options.Host)
	}

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("pgx.ConnectConfig: %v", err)
	}

	return conn, nil
}

func NewPostgresClientFromDBInstance(db *pgx.Conn) PostgresClient {
	return &pgClient{Conn: db}
}

// type pgReporter
type pgClient struct {
	*pgx.Conn
}

func (p *pgClient) DailyUsage(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error) {
	ctx := context.Background()
	// TODO: delegate dealing with the timestamps to the sql query: looks like there is a bug in QueryContext in dealing with parameters
	q := fmt.Sprintf("SELECT (time, application, count_success, count_failure) FROM daily_app_sums as d WHERE d.time >= '%s' and d.time <= '%s'",
		from.Format(dayLayout),
		to.Format(dayLayout),
	)
	rows, err := p.Conn.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

		// TODO: remove this after we get a better way to ensure that the layout will match in different timezones
		if strings.HasSuffix(items[0], "Z") {
			items[0] = strings.Replace(items[0], "Z", "+00", 1)
		}
		if !strings.HasSuffix(items[0], "+00") {
			items[0] = items[0][:len(items[0])-3] + "+00"
		}

		ts, err := time.Parse(pgTimeFormat, items[0])
		if err != nil {
			return nil, fmt.Errorf("Invalid time format: %s in query result line: %s, error: %v", items[0], r, err)
		}
		countSuccess, err := strconv.ParseInt(items[2], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		countFailure, err := strconv.ParseInt(items[3], 10, 64) // bitsize 64 for int64 return
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
		dailyUsage[ts][app] = api.RelayCounts{Success: countSuccess, Failure: countFailure}
	}

	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return dailyUsage, nil
}

func (p *pgClient) WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.Conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}

	// TODO: bulk insert
	for day, appCounts := range counts {
		for app, counts := range appCounts {
			_, execErr := tx.Exec(ctx,
				"INSERT INTO daily_app_sums(application, count_success, count_failure, time) VALUES($1, $2, $3, $4);",
				app, counts.Success, counts.Failure, day)
			if execErr != nil {
				if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
					fmt.Printf("update failed err write dailyUsage: %v, unable to rollback: %v\n", execErr, rollbackErr.Error())
					return execErr
				}
				fmt.Printf("update failed err write dailyUsage: %v", execErr.Error())
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (p *pgClient) ExistingMetricsTimespan() (time.Time, time.Time, error) {
	ctx := context.Background()
	row := p.Conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*), COALESCE(min(time), '2003-01-02 03:04' ), COALESCE(max(time), '2003-01-02 03:04') FROM %s", tableDailySums))
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

func (p *pgClient) WriteTodaysMetrics(counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts, latencies map[string][]api.Latency) error {
	ctx := context.Background()
	// TODO: determine required isolation level
	tx, err := p.Conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}

	err = p.writeTodaysLatency(ctx, tx, latencies)
	if err != nil {
		return fmt.Errorf("error writing latency: %s", err.Error())
	}

	err = p.WriteTodaysUsage(ctx, tx, counts, countsOrigin)
	if err != nil {
		return fmt.Errorf("error writing usage: %s", err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// WriteTodaysUsage writes the app metrics for today so far to the underlying PG table.
//
//	All the entries in the table holding todays metrics are deleted first.
func (p *pgClient) WriteTodaysUsage(ctx context.Context, tx pgx.Tx, counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error {
	if err := WriteAppUsage(ctx, tx, counts); err != nil {
		return err
	}

	if err := WriteOriginUsage(ctx, tx, countsOrigin); err != nil {
		return err
	}

	return nil
}

func WriteAppUsage(ctx context.Context, tx pgx.Tx, counts map[string]api.RelayCounts) error {
	// todays_sums table gets rebuilt every time
	_, deleteErr := tx.Exec(ctx, "DELETE FROM todays_app_sums")
	if deleteErr != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			fmt.Printf("delete failed: %v, unable to rollback: %v\n", deleteErr, rollbackErr.Error())
			return deleteErr
		}
	}

	// TODO: bulk insert
	for app, count := range counts {
		_, execErr := tx.Exec(ctx,
			"INSERT INTO todays_app_sums(application, count_success, count_failure) VALUES($1, $2, $3);",
			app, count.Success, count.Failure)
		if execErr != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				fmt.Printf("update failed err writeAppUsage: %v, unable to rollback: %v\n", execErr, rollbackErr.Error())
				return execErr
			}
			fmt.Printf("update failed err writeAppUsage: %v", execErr.Error())
		}
	}

	return nil
}

func WriteOriginUsage(ctx context.Context, tx pgx.Tx, counts map[string]api.RelayCounts) error {
	// todays_sums table gets rebuilt every time
	_, deleteErr := tx.Exec(ctx, "DELETE FROM todays_relay_counts")
	if deleteErr != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			fmt.Printf("delete failed: %v, unable to rollback: %v\n", deleteErr, rollbackErr)
			return deleteErr
		}
	}

	// TODO: bulk insert
	for origin, count := range counts {
		_, execErr := tx.Exec(ctx,
			"INSERT INTO todays_relay_counts(origin, count_success, count_failure) VALUES($1, $2, $3);",
			origin, count.Success, count.Failure)
		if execErr != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				fmt.Printf("update failed err write origin usage: %v, unable to rollback: %v\n", execErr, rollbackErr.Error())
				return execErr
			}
			fmt.Printf("update failed err write origin usage: %v", execErr.Error())
		}
	}

	return nil
}

// WriteTodaysUsage writes the app metrics for today so far to the underlying PG table.
//
//	All the entries in the table holding todays metrics are deleted first.
func (p *pgClient) writeTodaysLatency(ctx context.Context, tx pgx.Tx, latencies map[string][]api.Latency) error {
	// todays_app_latencies table gets rebuilt every time
	_, deleteErr := tx.Exec(ctx, "DELETE FROM todays_app_latencies")
	if deleteErr != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			fmt.Printf("delete failed: %v, unable to rollback: %v\n", deleteErr, rollbackErr)
			return deleteErr
		}
	}

	// TODO: bulk insert
	for app, appLatency := range latencies {
		for _, appLatency := range appLatency {
			_, execErr := tx.Exec(ctx,
				"INSERT INTO todays_app_latencies(application, time, latency) VALUES($1, $2, $3);",
				app, appLatency.Time, appLatency.Latency)

			if execErr != nil {
				if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
					fmt.Printf("update failed err write today latency: %v, unable to rollback: %v\n", execErr, rollbackErr.Error())
					return execErr
				}
				fmt.Printf("update failed err write today latency: %v", execErr.Error())
			}
		}
	}

	return nil
}

// TodaysUsage returns the current day's metrics so far.
func (p *pgClient) TodaysUsage() (map[string]api.RelayCounts, error) {
	// TODO: factor-out the SQL statements
	ctx := context.Background()
	rows, err := p.Conn.Query(ctx, "SELECT (application, count_success, count_failure) FROM todays_app_sums")
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

		countSuccess, err := strconv.ParseInt(items[1], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[1], r, err)
		}
		countFailure, err := strconv.ParseInt(items[2], 10, 64) // bitsize 64 for int64 return
		if err != nil {
			return nil, fmt.Errorf("Invalid total relays format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		app := items[0]
		if app == "" {
			return nil, fmt.Errorf("Empty application public key, in query result line: %s", r)
		}

		todaysUsage[app] = api.RelayCounts{Success: countSuccess, Failure: countFailure}
	}

	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return todaysUsage, nil
}

// TodaysLatency returns the past 24 hours' latency per app.
func (p *pgClient) TodaysLatency() (map[string][]api.Latency, error) {
	// TODO: factor-out the SQL statements
	ctx := context.Background()
	rows, err := p.Conn.Query(ctx, "SELECT (application, time, latency) FROM todays_app_latencies")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todaysLatency := make(map[string][]api.Latency)

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

		// TODO: remove this after we get a better way to ensure that the layout will match in different timezones
		if strings.HasSuffix(items[1], "Z") {
			items[1] = strings.Replace(items[1], "Z", "+00", 1)
		}
		if !strings.HasSuffix(items[1], "+00") {
			items[1] = items[1][:len(items[1])-3] + "+00"
		}

		hourlyTime, err := time.Parse(pgTimeFormat, items[1])
		if err != nil {
			return nil, fmt.Errorf("Invalid latency time format: %s in query result line: %s, error: %v", items[1], r, err)
		}
		hourlyAverageLatency, err := strconv.ParseFloat(items[2], 32)
		if err != nil {
			return nil, fmt.Errorf("Invalid latency format: %s in query result line: %s, error: %v", items[2], r, err)
		}
		app := items[0]
		if app == "" {
			return nil, fmt.Errorf("Empty application public key, in query result line: %s", r)
		}

		latencyByHour := api.Latency{Time: hourlyTime, Latency: numbers.RoundFloat(hourlyAverageLatency, 5)}

		todaysLatency[app] = append(todaysLatency[app], latencyByHour)

	}

	// Rows.Err will report the last error encountered by Rows.Scan.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return todaysLatency, nil
}

// TodaysUsage returns the current day's metrics so far.
func (pg *pgClient) TodaysOriginUsage() (map[string]api.RelayCounts, error) {
	// TODO: factor-out the SQL statements
	ctx := context.Background()
	rows, err := pg.Conn.Query(ctx, "SELECT (origin, count_success, count_failure) FROM todays_relay_counts")
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
		origin := items[0]
		if origin == "" {
			return nil, fmt.Errorf("Empty origin, in query result line: %s", r)
		}

		todaysUsage[origin] = api.RelayCounts{
			Success: count_success,
			Failure: count_failure,
		}
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
