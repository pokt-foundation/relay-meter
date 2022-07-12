package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	logger "github.com/sirupsen/logrus"
)

const (
	dayFormat = "2006-01-02"

	// TODO: make all time-related parameters configurable
	TTL_DAILY_METRICS_SECONDS  = 900
	TTL_TODAYS_METRICS_SECONDS = 600

	MAX_PAST_DAYS_METRICS = 30
)

type RelayMeter interface {
	// AppRelays returns total number of relays for the app over the specified time period
	AppRelays(app string, from, to time.Time) (AppRelaysResponse, error)
	// TODO: relays(user, timePeriod): returns total number of relays for all apps of the user over the specified time period (granularity roughly 1 day as a starting point)
	// TODO: totalrelays(timePeriod)
}

type Backend interface {
	//TODO: reverse map keys order, i.e. map[app]-> map[day]int64, at PG level
	DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error)
	TodaysUsage() (map[string]int64, error)
}

func NewRelayMeter(backend Backend, logger *logger.Logger, loadInterval time.Duration) RelayMeter {
	// PG client
	meter := &relayMeter{
		Backend: backend,
		Logger:  logger,
	}
	go func() { meter.StartDataLoader(context.Background(), loadInterval) }()
	return meter
}

// TODO: Add Cache
type relayMeter struct {
	Backend
	*logger.Logger

	dailyUsage  map[time.Time]map[string]int64
	todaysUsage map[string]int64

	dailyTTL  time.Time
	todaysTTL time.Time
	rwMutex   sync.RWMutex
}

// TODO: for now, today's data gets overwritten every time. If needed add todays metrics in intervals as they occur in the day
func (r *relayMeter) loadData(from, to time.Time) error {
	var updateDaily, updateToday bool

	now := time.Now()
	var dailyUsage map[time.Time]map[string]int64
	var todaysUsage map[string]int64
	var err error
	if now.After(r.dailyTTL) {
		updateDaily = true
		// TODO: send backend requests concurrently
		dailyUsage, err = r.Backend.DailyUsage(from, to)
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading daily usage data")
			return err
		}
	}

	if now.After(r.todaysTTL) {
		updateToday = true
		todaysUsage, err = r.Backend.TodaysUsage()
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading todays usage data")
			return err
		}
	}

	if !updateDaily && !updateToday {
		return nil
	}

	r.rwMutex.Lock()
	defer r.rwMutex.Unlock()

	if updateDaily {
		r.dailyUsage = dailyUsage
		r.dailyTTL = time.Now().Add(time.Duration(TTL_DAILY_METRICS_SECONDS) * time.Second)
	}
	if updateToday {
		r.todaysUsage = todaysUsage
		r.todaysTTL = time.Now().Add(time.Duration(TTL_TODAYS_METRICS_SECONDS) * time.Second)
	}
	return nil
}

// TODO: add a cache library, e.g. bigcache, if necessary (a cache library may not be needed, as we have a few thousand apps, for a maximum of 30 days)
// Notes on To and From parameters:
// Both parameters are assumed to be in the same timezone as the source of the data, i.e. influx
//	The From parameter is taken to mean the very start of the day that it specifies: the returned result includes all such relays
func (r *relayMeter) AppRelays(app string, from, to time.Time) (AppRelaysResponse, error) {
	resp := AppRelaysResponse{
		From:        from,
		To:          to,
		Application: app,
	}

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, to, err := AdjustTimePeriod(from, to)
	if err != nil {
		return resp, err
	}

	// Get today's date in day-only format
	now := time.Now()
	_, today, _ := AdjustTimePeriod(now, now)

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	var total int64
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			total += counts[app]
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		total += r.todaysUsage[app]
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	return resp, nil
}

// Starts a data loader in a go routine, to periodically load data from the backend
// 	context allows stopping the data loader
func (r *relayMeter) StartDataLoader(ctx context.Context, loadInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(loadInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				from := time.Now().Add(-24 * time.Hour * time.Duration(MAX_PAST_DAYS_METRICS))
				from, to, err := AdjustTimePeriod(from, time.Now())
				if err != nil {
					r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error setting timespan for data loader")
					break
				}
				r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("Starting data loader...")
				if err := r.loadData(from, to); err != nil {
					r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error setting timespan for data loader")
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}

// AdjustTimePeriod sets the two parameters, i.e. from and to, according to the following rules:
//	- From is adjusted to the start of the day that it originally specifies
//	- To is adjusted to the start of the next day from the day it originally specifies
func AdjustTimePeriod(from, to time.Time) (time.Time, time.Time, error) {
	if !from.Before(to) && !from.Equal(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid timespan: %v -- %v", from, to)
	}

	from, err := time.Parse(dayFormat, from.Format(dayFormat))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	to, err = time.Parse(dayFormat, to.Format(dayFormat))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to.AddDate(0, 0, 1), nil
}
