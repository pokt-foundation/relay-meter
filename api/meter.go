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
	TTL_DAILY_METRICS_DEFAULT_SECONDS  = 900
	TTL_TODAYS_METRICS_DEFAULT_SECONDS = 600

	MAX_PAST_DAYS_METRICS_DEFAULT_DAYS = 30
)

type RelayMeter interface {
	// AppRelays returns total number of relays for the app over the specified time period
	AppRelays(app string, from, to time.Time) (AppRelaysResponse, error)
	// TODO: relays(user, timePeriod): returns total number of relays for all apps of the user over the specified time period (granularity roughly 1 day as a starting point)
	UserRelays(user string, from, to time.Time) (UserRelaysResponse, error)
	TotalRelays(from, to time.Time) (TotalRelaysResponse, error)
}

// TODO: refactor common fields
type AppRelaysResponse struct {
	Count       int64
	From        time.Time
	To          time.Time
	Application string
}

type UserRelaysResponse struct {
	Count        int64
	From         time.Time
	To           time.Time
	User         string
	Applications []string
}

type TotalRelaysResponse struct {
	Count int64
	From  time.Time
	To    time.Time
}

type RelayMeterOptions struct {
	LoadInterval     time.Duration
	DailyMetricsTTL  time.Duration
	TodaysMetricsTTL time.Duration
	MaxPastDays      time.Duration
}

type Backend interface {
	//TODO: reverse map keys order, i.e. map[app]-> map[day]int64, at PG level
	DailyUsage(from, to time.Time) (map[time.Time]map[string]int64, error)
	TodaysUsage() (map[string]int64, error)
	// Is expected to return the list of applicationIDs owned by the user
	UserApps(user string) ([]string, error)
}

func NewRelayMeter(backend Backend, logger *logger.Logger, options RelayMeterOptions) RelayMeter {
	// PG client
	meter := &relayMeter{
		Backend:           backend,
		Logger:            logger,
		RelayMeterOptions: options,
	}
	go func() { meter.StartDataLoader(context.Background()) }()
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

	RelayMeterOptions
}

func (r *relayMeter) isEmpty() bool {
	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	return len(r.dailyUsage) == 0 || len(r.todaysUsage) == 0
}

// TODO: for now, today's data gets overwritten every time. If needed add todays metrics in intervals as they occur in the day
func (r *relayMeter) loadData(from, to time.Time) error {
	var updateDaily, updateToday bool

	now := time.Now()
	var dailyUsage map[time.Time]map[string]int64
	var todaysUsage map[string]int64
	var err error
	noDataYet := r.isEmpty()

	if noDataYet || now.After(r.dailyTTL) {
		updateDaily = true
		// TODO: send backend requests concurrently
		dailyUsage, err = r.Backend.DailyUsage(from, to)
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading daily usage data")
			return err
		}
		r.Logger.WithFields(logger.Fields{"daily_metrics_count": len(dailyUsage)}).Info("Received daily metrics")
	}

	if noDataYet || now.After(r.todaysTTL) {
		updateToday = true
		todaysUsage, err = r.Backend.TodaysUsage()
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading todays usage data")
			return err
		}
		r.Logger.WithFields(logger.Fields{"todays_metrics_count": len(todaysUsage)}).Info("Received todays metrics")
	}

	if !updateDaily && !updateToday {
		return nil
	}

	r.rwMutex.Lock()
	defer r.rwMutex.Unlock()

	if updateDaily {
		r.dailyUsage = dailyUsage
		d := r.RelayMeterOptions.DailyMetricsTTL
		if int(d.Seconds()) == 0 {
			d = time.Duration(TTL_DAILY_METRICS_DEFAULT_SECONDS) * time.Second
		}
		r.dailyTTL = time.Now().Add(d)
	}
	if updateToday {
		r.todaysUsage = todaysUsage
		d := r.RelayMeterOptions.TodaysMetricsTTL
		if int(d.Seconds()) == 0 {
			d = time.Duration(TTL_TODAYS_METRICS_DEFAULT_SECONDS) * time.Second
		}
		r.todaysTTL = time.Now().Add(d)
	}
	return nil
}

// TODO: add a cache library, e.g. bigcache, if necessary (a cache library may not be needed, as we have a few thousand apps, for a maximum of 30 days)
// Notes on To and From parameters:
// Both parameters are assumed to be in the same timezone as the source of the data, i.e. influx
//	The From parameter is taken to mean the very start of the day that it specifies: the returned result includes all such relays
func (r *relayMeter) AppRelays(app string, from, to time.Time) (AppRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"app": app, "from": from, "to": to}).Info("apiserver: Received AppRelays request")
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

// TODO: refactor the common processing done by both AppRelays and UserRelays
func (r *relayMeter) UserRelays(user string, from, to time.Time) (UserRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"user": user, "from": from, "to": to}).Info("apiserver: Received UserRelays request")
	resp := UserRelaysResponse{
		From: from,
		To:   to,
		User: user,
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

	apps, err := r.Backend.UserApps(user)
	if err != nil {
		return resp, err
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	var total int64
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			for _, app := range apps {
				total += counts[app]
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, app := range apps {
			total += r.todaysUsage[app]
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to
	resp.Applications = apps

	return resp, nil
}

func (r *relayMeter) TotalRelays(from, to time.Time) (TotalRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received TotalRelays request")
	resp := TotalRelaysResponse{
		From: from,
		To:   to,
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
			for _, count := range counts {
				total += count
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, count := range r.todaysUsage {
			total += count
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	return resp, nil
}

// Starts a data loader in a go routine, to periodically load data from the backend
// 	context allows stopping the data loader
func (r *relayMeter) StartDataLoader(ctx context.Context) {
	maxPastDays := maxArchiveAge(r.RelayMeterOptions.MaxPastDays)

	load := func(max time.Duration) {
		from := time.Now().Add(max)
		from, to, err := AdjustTimePeriod(from, time.Now())
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error setting timespan for data loader")
			return
		}
		r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("Starting data loader...")
		if err := r.loadData(from, to); err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error setting timespan for data loader")
		}
	}

	r.Logger.WithFields(logger.Fields{"maxArchiveAge": maxPastDays}).Info("Running initial data loader iteration...")
	load(maxPastDays)
	go func(maxDays time.Duration) {
		ticker := time.NewTicker(r.RelayMeterOptions.LoadInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				load(maxDays)
			}
		}
	}(maxPastDays)

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

	// TODO: refactor: there is some duplication in the function
	getDefault := func(parameter time.Time, defaultValue time.Time) (time.Time, error) {
		if parameter.Equal(time.Time{}) {
			return time.Parse(dayFormat, defaultValue.Format(dayFormat))
		}
		return parameter, nil
	}

	var err error
	// TODO: set default from parameter to the actual MaxPastDays passed to the meter, i.e. r.RelayMeterOptions.MaxPastDays
	from, err = getDefault(from, time.Now().Add(-24*time.Hour*time.Duration(MAX_PAST_DAYS_METRICS_DEFAULT_DAYS)))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	// Missing 'to' is set to include today
	to, err = getDefault(to, time.Now())
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if !from.Before(to) && !from.Equal(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid timespan: %v -- %v", from, to)
	}

	from, err = time.Parse(dayFormat, from.Format(dayFormat))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	to, err = time.Parse(dayFormat, to.Format(dayFormat))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to.AddDate(0, 0, 1), nil
}

func maxArchiveAge(maxPastDays time.Duration) time.Duration {
	if maxPastDays == 0 {
		return -24 * time.Hour * time.Duration(MAX_PAST_DAYS_METRICS_DEFAULT_DAYS)
	}
	return time.Duration(-1) * maxPastDays
}
