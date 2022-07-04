package api

import (
	"fmt"
	"time"

	logger "github.com/sirupsen/logrus"
)

const (
	dayFormat = "2006-01-02"
)

type Backend interface {
	//TODO: reverse map keys order, i.e. map[app]-> map[day]int64, at PG level
	DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error)
}

func NewRelayMeter(backend Backend, logger *logger.Logger) RelayMeter {
	// PG client
	return &relayMeter{
		Backend: backend,
		Logger:  logger,
	}
}

// TODO: Add Cache
type relayMeter struct {
	Backend
	*logger.Logger

	lastQueryTime time.Time
	dailyUsage    map[time.Time]map[string]int64
}

// TODO: for now, today's data gets overwritten every time. If needed, have a separate table for today's relays, adding periods as they occur in the day
func (r *relayMeter) loadData(from, to time.Time) error {
	usage, err := r.Backend.DailyUsage(from, to)
	if err != nil {
		return err
	}
	r.dailyUsage = usage
	return nil
}

// Notes on To and From parameters:
// Both parameters are assumed to be in the same timezone as the source of the data, i.e. influx
//	The From parameter is taken to mean the very start of the day that it specifies: the returned result includes all such relays
func (r *relayMeter) AppRelays(app string, from, to time.Time) (AppRelaysResponse, error) {
	resp := AppRelaysResponse{
		From:        from,
		To:          to,
		Application: app,
	}
	if !from.Before(to) && !from.Equal(to) {
		return resp, fmt.Errorf("Invalid timespan: %v -- %v", from, to)
	}

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, err := time.Parse(dayFormat, from.Format(dayFormat))
	if err != nil {
		return resp, err
	}

	// TODO: adjust To
	to, err = time.Parse(dayFormat, to.Format(dayFormat))
	if err != nil {
		return resp, err
	}

	// TODO: simple TTL: just query once everty 5 minutes
	if err := r.loadData(from, to); err != nil {
		return resp, fmt.Errorf("Error loading data")
	}

	var total int64
	for day, counts := range r.dailyUsage {
		if (day.After(from) || day.Equal(from)) && (day.Before(to) || day.Equal(to)) {
			total += counts[app]
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	// TODO: Add current day's usage
	return resp, nil
}
