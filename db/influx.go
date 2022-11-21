package db

// TODO: do we need a more secure way of passing the passwords?
import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

	"github.com/adshmh/meter/api"
)

type Source interface {
	AppRelays(from, to time.Time) (map[string]api.RelayCounts, error)
	DailyCounts(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error)
	// Returns application metrics for today so far
	TodaysCounts() (map[string]api.RelayCounts, error)
	// Returns relay counts for today grouped by origin
	TodaysCountsPerOrigin() (map[string]int64, error)
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
//
//	Each app will have an entry per day
func (i *influxDB) DailyCounts(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI(i.Options.Org)

	// Loop on days

	dailyCounts := make(map[time.Time]map[string]api.RelayCounts)
	// TODO: the influx doc seems to have a bug when describing the 'stop' parameter of range function,
	//	i.e. it says "Results exclude rows with _time values that match the specified start time.", likely meant to say 'stop time'
	//	https://docs.influxdata.com/flux/v0.x/stdlib/universe/range/
	// 	--> this needs verification to make sure we do not double-count the last second of each day

	// TODO: send queries in parallel
	// TODO: use 'sum' in the influx query to reduce the number of returned data points
	for current := from; current.Before(to); current = current.AddDate(0, 0, 1) {
		query := fmt.Sprintf("from(bucket: %q)", i.Options.DailyBucket) +
			fmt.Sprintf(" |> range(start: %s, stop: %s)", current.Format(time.RFC3339), current.AddDate(0, 0, 1).Format(time.RFC3339)) +
			fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_measurement", "relay") +
			fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_field", "count") +
			fmt.Sprintf(" |> group(columns: [%q, %q])", "applicationPublicKey", "result") +
			fmt.Sprintf(" |> keep(columns: [%q, %q, %q])", "applicationPublicKey", "result", "_value")

		result, err := queryAPI.Query(context.Background(), query)
		if err != nil {
			return nil, err
		}

		counts := make(map[string]api.RelayCounts)
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

			count, ok := result.Record().Value().(int64)
			if !ok {
				return nil, fmt.Errorf("Error parsing application %s relay counts %v", app, result.Record().Value())
			}

			// TODO: log app + count + time
			relayResult, ok := result.Record().ValueByKey("result").(string)
			if !ok {
				return nil, fmt.Errorf("Error parsing relay result: %v", result.Record().ValueByKey("result"))
			}
			updatedCount, err := updateRelayCount(counts[app], relayResult, count)
			if err != nil {
				return nil, err
			}
			counts[app] = updatedCount
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

// TODO: Refactor out the parts of the logic common between TodaysCounts and DailyCounts
func (i *influxDB) TodaysCounts() (map[string]api.RelayCounts, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI(i.Options.Org)

	counts := make(map[string]api.RelayCounts)
	// TODO: send queries in parallel
	query := fmt.Sprintf("from(bucket: %q)", i.Options.CurrentBucket) +
		fmt.Sprintf(" |> range(start: %s)", startOfDay(time.Now()).Format(time.RFC3339)) +
		fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_measurement", "relay") +
		fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_field", "count") +
		fmt.Sprintf(" |> keep(columns: [%q, %q, %q])", "applicationPublicKey", "result", "_value") +
		fmt.Sprintf(" |> group(columns: [%q, %q])", "applicationPublicKey", "result") +
		fmt.Sprintf(" |> sum()")

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

		count, ok := result.Record().Value().(int64)
		if !ok {
			return nil, fmt.Errorf("Error parsing application %s relay counts %v", app, result.Record().Value())
		}

		// TODO: log app + count + time
		relayResult, ok := result.Record().ValueByKey("result").(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing relay result: %v", result.Record().ValueByKey("result"))
		}
		updatedCount, err := updateRelayCount(counts[app], relayResult, count)
		if err != nil {
			return nil, err
		}
		counts[app] = updatedCount
	}
	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

func (i *influxDB) TodaysCountsPerOrigin() (map[string]int64, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI(i.Options.Org)

	counts := make(map[string]int64)
	// TODO: send queries in parallel
	query := fmt.Sprintf("from(bucket: %q)", i.Options.CurrentBucket) +
		fmt.Sprintf(" |> range(start: %s,stop: %s)", startOfDay(time.Now()).Format(time.RFC3339), time.Now().Format(time.RFC3339)) +
		fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_measurement", "origin") +
		fmt.Sprintf(" |> filter(fn: (r) => r[%q] == %q)", "_field", "count") +
		fmt.Sprintf(" |> group(columns: [%q])", "origin") +
		" |> aggregateWindow(every: 24h, fn:mean, createEmpty: false)" +
		fmt.Sprintf(" |> yield(name:%q)", "mean")

	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	// Iterate over query response
	for result.Next() {
		origin, ok := result.Record().ValueByKey("origin").(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing application origin: %v", result.Record().ValueByKey("origin"))
		}
		// TODO: log a warning on empty app key
		if origin == "" {
			fmt.Println("Warning: empty origin in relay")
			continue
		}

		count, ok := result.Record().Value().(int64)
		if !ok {
			return nil, fmt.Errorf("Error parsing origin %s relay counts %v", origin, result.Record().Value())
		}

		if counts[origin] == 0 {
			counts[origin] = count
			continue
		}

		counts[origin] += count
	}

	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

func updateRelayCount(current api.RelayCounts, relayResult string, count int64) (api.RelayCounts, error) {
	switch {
	case relayResult == fmt.Sprintf("%d", http.StatusOK):
		current.Success += int64(count)
	case relayResult == fmt.Sprintf("%d", http.StatusInternalServerError):
		current.Failure += int64(count)
	default:
		return api.RelayCounts{}, fmt.Errorf("Invalid value in result field: %s", relayResult)
	}
	return current, nil
}

// TODO: Remove this and all references.
func (i *influxDB) AppRelays(from, to time.Time) (map[string]api.RelayCounts, error) {
	// Create a new client using an InfluxDB server base URL and an authentication token
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	// Get query client
	queryAPI := client.QueryAPI(i.Options.Org)

	query := `from(bucket:"relays")|> range(` + fmt.Sprintf("start: %d,", from.Unix()) + fmt.Sprintf("stop: %d)", to.Unix()) + ` |> filter(fn: (r) => r._measurement == "relay") |> group(columns: ["applicationPublicKey"]) |> count()`

	result, err := queryAPI.Query(context.Background(), query)

	if err != nil {
		return nil, err
	}

	counts := make(map[string]api.RelayCounts)

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
		counts[app] = api.RelayCounts{Success: count}
	}

	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

// startOfDay returns the time matching the start of the day of the input.
//
//	timezone/location is maintained.
func startOfDay(day time.Time) time.Time {
	y, m, d := day.Date()
	l := day.Location()

	return time.Date(y, m, d, 0, 0, 0, 0, l)
}
