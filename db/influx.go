package db

// TODO: do we need a more secure way of passing the passwords?
import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/pokt-foundation/utils-go/numbers"
	timeUtils "github.com/pokt-foundation/utils-go/time"

	"github.com/pokt-foundation/relay-meter/api"
)

type Source interface {
	AppRelays(from, to time.Time) (map[string]api.RelayCounts, error)
	DailyCounts(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error)
	// Returns application metrics for today so far
	TodaysCounts() (map[string]api.RelayCounts, error)
	// Returns relay counts for today grouped by origin
	TodaysCountsPerOrigin() (map[string]api.RelayCounts, error)
	TodaysLatency() (map[string][]api.Latency, error)
	Name() string
}

type InfluxDBOptions struct {
	URL   string
	Token string
	Org   string
	// Bucket to query for previous days counts
	DailyBucket string
	// Bucket to query for today's counts
	CurrentBucket string
	// Bucket to query for previous days counts per origin
	DailyOriginBucket string
	// Bucket to query for today's counts per origin
	CurrentOriginBucket string
}

func NewInfluxDBSource(options InfluxDBOptions) Source {
	return &influxDB{Options: options}
}

type influxDB struct {
	Options InfluxDBOptions
}

func (*influxDB) Name() string {
	return "influx"
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
		queryString := `from(bucket: %q)
		  |> range(start: %s, stop: %s)
		  |> filter(fn: (r) => r["_measurement"] == "relay")
		  |> filter(fn: (r) => r["_field"] == "count")
		  |> group(columns: ["applicationPublicKey", "result"])
		  |> keep(columns: ["applicationPublicKey", "result", "_value"])`
		fluxQuery := fmt.Sprintf(queryString, i.Options.DailyBucket, current.Format(time.RFC3339), current.AddDate(0, 0, 1).Format(time.RFC3339))

		result, err := queryAPI.Query(context.Background(), fluxQuery)
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
	queryString := `from(bucket: %q)
	  |> range(start: %s)
	  |> filter(fn: (r) => r["_measurement"] == "relay")
	  |> filter(fn: (r) => r["_field"] == "count")
	  |> keep(columns: ["applicationPublicKey", "result", "_value"])
	  |> group(columns: ["applicationPublicKey", "result"])
	  |> sum()`
	fluxQuery := fmt.Sprintf(queryString, i.Options.CurrentBucket, timeUtils.StartOfDay(time.Now()).Format(time.RFC3339))

	result, err := queryAPI.Query(context.Background(), fluxQuery)
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

func (i *influxDB) TodaysCountsPerOrigin() (map[string]api.RelayCounts, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI(i.Options.Org)

	counts := make(map[string]api.RelayCounts)
	// TODO: send queries in parallel

	queryString := `from(bucket: %q)
	 |> range(start: %s,stop: %s)
	 |> filter(fn: (r) => r["_measurement"] == "origin")
	 |> filter(fn: (r) => r["_field"] == "count")
	 |> keep(columns: ["origin","result","_value"])
	 |> group(columns: ["origin","result"])
	 |> sum()
	`
	query := fmt.Sprintf(queryString, i.Options.CurrentOriginBucket, timeUtils.StartOfDay(time.Now()).Format(time.RFC3339), time.Now().Format(time.RFC3339))

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

		// TODO: log app + count + time
		relayResult, ok := result.Record().ValueByKey("result").(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing relay result: %v", result.Record().ValueByKey("result"))
		}

		updatedCount, err := updateRelayCountOrigin(counts[origin], relayResult, count)
		if err != nil {
			return nil, err
		}

		counts[origin] = updatedCount
	}

	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()
	return counts, nil
}

// Fetches the last 24 hours of latency data from InfluxDB, sorted by applicationPublicKey
// and broken up into hourly average latency (returned slice will be exactly 24 items)
func (i *influxDB) TodaysLatency() (map[string][]api.Latency, error) {
	client := influxdb2.NewClient(i.Options.URL, i.Options.Token)
	queryAPI := client.QueryAPI(i.Options.Org)

	latencies := make(map[string][]api.Latency)

	// TODO: send queries in parallel
	oneDayAgo := time.Now().Add(-time.Hour * 24).Format(time.RFC3339)
	queryString := `from(bucket: %q)
	  |> range(start: %s)
	  |> filter(fn: (r) => r["_measurement"] == "relay")
	  |> filter(fn: (r) => r["_field"] == "elapsedTime")
	  |> group(columns: ["host", "applicationPublicKey", "region", "result", "method"])
	  |> keep(columns: ["applicationPublicKey", "_time", "_value"])
	  |> aggregateWindow(every: 1h, fn: mean)`
	fluxQuery := fmt.Sprintf(queryString, i.Options.CurrentBucket, oneDayAgo)

	result, err := queryAPI.Query(context.Background(), fluxQuery)
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

		if len(latencies[app]) < 24 {
			hourlyTime, ok := result.Record().ValueByKey("_time").(time.Time)
			if !ok {
				return nil, fmt.Errorf("Error parsing latency time: %v", result.Record().ValueByKey("_time"))
			}

			hourlyAverageLatency := float64(0)
			if result.Record().ValueByKey("_value") != nil {
				hourlyAverageLatency, ok = result.Record().ValueByKey("_value").(float64)
				if !ok {
					return nil, fmt.Errorf("Error parsing latency time: %v", result.Record().ValueByKey("_value"))
				}

			}

			latencyByHour := api.Latency{Time: hourlyTime, Latency: numbers.RoundFloat(hourlyAverageLatency, 5)}

			latencies[app] = append(latencies[app], latencyByHour)
		}
	}

	// check for an error
	if result.Err() != nil {
		return nil, fmt.Errorf("query parsing error: %s", result.Err().Error())
	}

	client.Close()

	return latencies, nil
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

func updateRelayCountOrigin(current api.RelayCounts, relayResult string, count int64) (api.RelayCounts, error) {
	switch {
	case relayResult == "_result":
		current.Success += int64(count)
	// TODO: Update the condition when relays fails after we define it
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
