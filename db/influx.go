package db

// TODO: do we need a more secure way of passing the passwords?
import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/influxdata/influxdb-client-go/v2"

	"github.com/adshmh/meter/api"
)

type Source interface {
	AppRelays(from, to time.Time) (map[string]int64, error)
	// UserRelays(period time.Duration) (map[string]int, error)
	DailyCounts(from, to time.Time) (map[time.Time]map[string]int64, error)
	// Returns application metrics for today so far
	TodaysCounts() (map[string]int64, error)
}

type InfluxDBOptions struct {
	URL   string
	Token string
	Org   string
	// Bucket to query for previous days counts
	DailyBucket string
	// Bucket to query for today's counts
	CurrentBucket string
}

func NewInfluxDBSource(options InfluxDBOptions) Source {
	return &influxDB{Options: options}
}

type influxDB struct {
	Options InfluxDBOptions
}

// DailyCounts Returns total of number of daily relays per application, up to and including the specified day
//	Each app will have an entry per day
func (i *influxDB) DailyCounts(from, to time.Time) (map[time.Time]map[string]int64, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI("my-org")

	// Loop on days
	startDay, endDay, err := api.AdjustTimePeriod(from, to)
	if err != nil {
		return nil, err
	}

	dailyCounts := make(map[time.Time]map[string]int64)
	// TODO: the influx doc seems to have a bug when describing the 'stop' parameter of range function,
	//	i.e. it says "Results exclude rows with _time values that match the specified start time.", likely meant to say 'stop time'
	//	https://docs.influxdata.com/flux/v0.x/stdlib/universe/range/
	// 	--> this needs verification to make sure we do not double-count the last second of each day

	// TODO: send queries in parallel
	for current := startDay; current.Before(endDay); current = current.AddDate(0, 0, 1) {
		query := fmt.Sprintf("from(bucket: %q)", i.Options.DailyBucket) +
			fmt.Sprintf(" |> range(start: %d, stop: %d)", current.Unix(), current.AddDate(0, 0, 1).Unix()) +
			fmt.Sprintf(" |> filter(fn: (r) => r._measurement == %q)", "relay") +
			fmt.Sprintf(" |> group(columns: [%q])", "applicationPublicKey") +
			" |> sum()"

		result, err := queryAPI.Query(context.Background(), query)
		if err != nil {
			return nil, err
		}

		counts := make(map[string]int64)
		// Iterate over query response
		for result.Next() {
			app, ok := result.Record().ValueByKey("applicationPublicKey").(string)
			if !ok {
				return nil, fmt.Errorf("Error parsing application public key: %v", result.Record().ValueByKey("applicationPublicKey"))
			}
			// TODO: log a warning on empty app key
			if app == "" {
				fmt.Println("Warning: empty application public key")
				continue
			}

			// Remove leading and trailing '"' from app
			app = strings.TrimPrefix(app, "\"")
			app = strings.TrimSuffix(app, "\"")

			count, ok := result.Record().Value().(float64)
			if !ok {
				return nil, fmt.Errorf("Error parsing application %s relay counts %v", app, result.Record().Value())
			}

			// TODO: log app + count + time
			counts[app] += int64(count)
		}
		// check for an error
		if result.Err() != nil {
			return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
		}
		dailyCounts[current] = counts
	}

	client.Close()
	return dailyCounts, nil
}

func (i *influxDB) TodaysCounts() (map[string]int64, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI("my-org")

	counts := make(map[string]int64)
	// TODO: send queries in parallel
	query := fmt.Sprintf("from(bucket: %q)", i.Options.CurrentBucket) +
		fmt.Sprintf(" |> range(start: %d)", startOfDay(time.Now()).Unix()) +
		fmt.Sprintf(" |> filter(fn: (r) => r._measurement == %q)", "relay") +
		fmt.Sprintf(" |> group(columns: [%q])", "applicationPublicKey") +
		" |> sum()"

	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	// Iterate over query response
	for result.Next() {
		app, ok := result.Record().ValueByKey("applicationPublicKey").(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing application public key: %v", result.Record().ValueByKey("applicationPublicKey"))
		}
		// TODO: log a warning on empty app key
		if app == "" {
			fmt.Println("Warning: empty application public key")
			continue
		}

		// Remove leading and trailing '"' from app
		app = strings.TrimPrefix(app, "\"")
		app = strings.TrimSuffix(app, "\"")

		count, ok := result.Record().Value().(float64)
		if !ok {
			return nil, fmt.Errorf("Error parsing application %s relay counts %v", app, result.Record().Value())
		}

		// TODO: log app + count + time
		counts[app] += int64(count)
	}
	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

func (i *influxDB) AppRelays(from, to time.Time) (map[string]int64, error) {
	// Create a new client using an InfluxDB server base URL and an authentication token
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	// Get query client
	queryAPI := client.QueryAPI("my-org")

	query := `from(bucket:"relays")|> range(` + fmt.Sprintf("start: %d,", from.Unix()) + fmt.Sprintf("stop: %d)", to.Unix()) + ` |> filter(fn: (r) => r._measurement == "relay") |> group(columns: ["applicationPublicKey"]) |> count()`

	result, err := queryAPI.Query(context.Background(), query)

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)

	// Iterate over query response
	for result.Next() {
		app, ok := result.Record().ValueByKey("applicationPublicKey").(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing application public key: %v", result.Record().ValueByKey("applicationPublicKey"))
		}
		// TODO: log a warning on empty app key
		if app == "" {
			fmt.Println("Warning: empty application public key")
		}

		// Remove leading and trailing '#' from app
		app = strings.TrimPrefix(app, "\"")
		app = strings.TrimSuffix(app, "\"")

		count, ok := result.Record().Value().(int64)
		if !ok {
			return nil, fmt.Errorf("Error parsing application %s relay counts %v", app, result.Record().Value())
		}
		// TODO: log app + count + time
		counts[app] = count
	}

	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

// startOfDay returns the time matching the start of the day of the input.
//	timezone/location is maintained.
func startOfDay(day time.Time) time.Time {
	y, m, d := day.Date()
	l := day.Location()

	return time.Date(y, m, d, 0, 0, 0, 0, l)
}
