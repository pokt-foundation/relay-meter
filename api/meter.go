package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/pokt-foundation/portal-api-go/repository"
)

const (
	dayFormat = "2006-01-02"

	// TODO: make all time-related parameters configurable
	TTL_DAILY_METRICS_DEFAULT_SECONDS  = 900
	TTL_TODAYS_METRICS_DEFAULT_SECONDS = 600

	MAX_PAST_DAYS_METRICS_DEFAULT_DAYS = 30
)

var (
	ErrLoadBalancerNotFound = errors.New("loadbalancer/endpoint not found")
	ErrAppLatencyNotFound   = errors.New("app latency not found")
)

type RelayMeter interface {
	// AppRelays returns total number of relays for the app over the specified time period
	AppRelays(app string, from, to time.Time) (AppRelaysResponse, error)
	AllAppsRelays(from, to time.Time) ([]AppRelaysResponse, error)
	UserRelays(user string, from, to time.Time) (UserRelaysResponse, error)
	TotalRelays(from, to time.Time) (TotalRelaysResponse, error)
	// LoadBalancerRelays returns the metrics for an Endpoint, AKA loadbalancer
	LoadBalancerRelays(endpoint string, from, to time.Time) (LoadBalancerRelaysResponse, error)
	AllLoadBalancersRelays(from, to time.Time) ([]LoadBalancerRelaysResponse, error)
	AppLatency(app string) (AppLatencyResponse, error)
	AllAppsLatencies() ([]AppLatencyResponse, error)
}

type RelayCounts struct {
	Success int64
	Failure int64
}

type Latency struct {
	Time    time.Time
	Latency float64
}

// TODO: refactor common fields
type AppRelaysResponse struct {
	Count       RelayCounts
	From        time.Time
	To          time.Time
	Application string
}

type AppLatencyResponse struct {
	DailyLatency []Latency
	From         time.Time
	To           time.Time
	Application  string
}

type UserRelaysResponse struct {
	Count        RelayCounts
	From         time.Time
	To           time.Time
	User         string
	Applications []string
}

type TotalRelaysResponse struct {
	Count RelayCounts
	From  time.Time
	To    time.Time
}

type LoadBalancerRelaysResponse struct {
	Count        RelayCounts
	From         time.Time
	To           time.Time
	Endpoint     string
	Applications []string
}

type RelayMeterOptions struct {
	LoadInterval     time.Duration
	DailyMetricsTTL  time.Duration
	TodaysMetricsTTL time.Duration
	MaxPastDays      time.Duration
}

type Backend interface {
	//TODO: reverse map keys order, i.e. map[app]-> map[day]RelayCounts, at PG level
	DailyUsage(from, to time.Time) (map[time.Time]map[string]RelayCounts, error)
	TodaysUsage() (map[string]RelayCounts, error)
	TodaysLatency() (map[string][]Latency, error)
	// Is expected to return the list of applicationIDs owned by the user
	UserApps(user string) ([]string, error)
	// LoadBalancer returns the full load balancer struct
	LoadBalancer(endpoint string) (*repository.LoadBalancer, error)
	LoadBalancers() ([]*repository.LoadBalancer, error)
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

	dailyUsage    map[time.Time]map[string]RelayCounts
	todaysUsage   map[string]RelayCounts
	todaysLatency map[string][]Latency

	dailyTTL  time.Time
	todaysTTL time.Time
	rwMutex   sync.RWMutex

	RelayMeterOptions
}

func (r *relayMeter) isEmpty() bool {
	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	return len(r.dailyUsage) == 0 || len(r.todaysUsage) == 0 || len(r.todaysLatency) == 0
}

// TODO: for now, today's data gets overwritten every time. If needed add todays metrics in intervals as they occur in the day
func (r *relayMeter) loadData(from, to time.Time) error {
	var updateDaily, updateToday bool

	now := time.Now()
	var dailyUsage map[time.Time]map[string]RelayCounts
	var todaysUsage map[string]RelayCounts
	var todaysLatency map[string][]Latency
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

		todaysLatency, err = r.Backend.TodaysLatency()
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading todays latency data")
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
		r.todaysLatency = todaysLatency

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

	var total RelayCounts
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			total.Success += counts[app].Success
			total.Failure += counts[app].Failure
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		total.Success += r.todaysUsage[app].Success
		total.Failure += r.todaysUsage[app].Failure
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	return resp, nil
}

func (r *relayMeter) AppLatency(app string) (AppLatencyResponse, error) {
	r.Logger.WithFields(logger.Fields{"app": app}).Info("apiserver: Received AppLatency request")

	appLatency := r.todaysLatency[app]

	if len(appLatency) == 0 {
		return AppLatencyResponse{}, ErrAppLatencyNotFound
	}

	sort.Slice(appLatency, func(i, j int) bool {
		return appLatency[i].Time.Before(appLatency[j].Time)
	})

	return AppLatencyResponse{
		Application:  app,
		DailyLatency: appLatency,
		From:         appLatency[0].Time,
		To:           appLatency[len(appLatency)-1].Time,
	}, nil
}

func (r *relayMeter) AllAppsLatencies() ([]AppLatencyResponse, error) {
	r.Logger.Info("apiserver: Received AllAppsLatencies request")

	resp := []AppLatencyResponse{}

	for app, appLatency := range r.todaysLatency {
		sort.Slice(appLatency, func(i, j int) bool {
			return appLatency[i].Time.Before(appLatency[j].Time)
		})

		latencyResp := AppLatencyResponse{
			Application:  app,
			DailyLatency: appLatency,
			From:         appLatency[0].Time,
			To:           appLatency[len(appLatency)-1].Time,
		}

		resp = append(resp, latencyResp)
	}

	return resp, nil
}

func (r *relayMeter) AllAppsRelays(from, to time.Time) ([]AppRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received AllAppRelays request")

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, to, err := AdjustTimePeriod(from, to)
	if err != nil {
		return nil, err
	}

	// Get today's date in day-only format
	now := time.Now()
	_, today, _ := AdjustTimePeriod(now, now)

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	rawResp := make(map[string]AppRelaysResponse)

	for day, counts := range r.dailyUsage {
		for pubKey, relCounts := range counts {
			total := rawResp[pubKey].Count

			// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
			if (day.After(from) || day.Equal(from)) && day.Before(to) {
				total.Success += relCounts.Success
				total.Failure += relCounts.Failure
			}

			rawResp[pubKey] = AppRelaysResponse{
				Application: pubKey,
				From:        from,
				To:          to,
				Count:       total,
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for pubKey, relCounts := range r.todaysUsage {
			total := rawResp[pubKey].Count

			total.Success += relCounts.Success
			total.Failure += relCounts.Failure

			rawResp[pubKey] = AppRelaysResponse{
				Application: pubKey,
				From:        from,
				To:          to,
				Count:       total,
			}
		}
	}

	resp := []AppRelaysResponse{}

	for _, relResp := range rawResp {
		resp = append(resp, relResp)
	}

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
		r.Logger.WithFields(logger.Fields{"user": user, "from": from, "to": to, "error": err}).Warn("Error getting user applications processing UserRelays request")
		return resp, err
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	var total RelayCounts
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			for _, app := range apps {
				total.Success += counts[app].Success
				total.Failure += counts[app].Failure
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, app := range apps {
			total.Success += r.todaysUsage[app].Success
			total.Failure += r.todaysUsage[app].Failure
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

	var total RelayCounts
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			for _, count := range counts {
				total.Success += count.Success
				total.Failure += count.Failure
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, count := range r.todaysUsage {
			total.Success += count.Success
			total.Failure += count.Failure
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	return resp, nil
}

// LoadBalancerRelays returns the metrics for all applications of a load balancer (AKA endpoint)
func (r *relayMeter) LoadBalancerRelays(endpoint string, from, to time.Time) (LoadBalancerRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"endpoint": endpoint, "from": from, "to": to}).Info("apiserver: Received LoadBalancerRelays request")
	resp := LoadBalancerRelaysResponse{
		From:     from,
		To:       to,
		Endpoint: endpoint,
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

	lb, err := r.Backend.LoadBalancer(endpoint)
	if err != nil {
		r.Logger.WithFields(logger.Fields{"endpoint": endpoint, "from": from, "to": to, "error": err}).Warn("Error getting endpoint/loadbalancer applications processing LoadBalancerRelays request")
		return resp, err
	}
	if lb == nil {
		return resp, ErrLoadBalancerNotFound
	}

	var apps []string
	for _, app := range lb.Applications {
		key := applicationPublicKey(app)
		if key != "" {
			apps = append(apps, key)
		}
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	var total RelayCounts
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			for _, app := range apps {
				total.Success += counts[app].Success
				total.Failure += counts[app].Failure
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, app := range apps {
			total.Success += r.todaysUsage[app].Success
			total.Failure += r.todaysUsage[app].Failure
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to
	resp.Applications = apps

	return resp, nil
}

// AllLoadBalancersRelays returns the metrics for all applications of all load balancers (AKA endpoints)
func (r *relayMeter) AllLoadBalancersRelays(from, to time.Time) ([]LoadBalancerRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received AllLoadBalancerRelays request")

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, to, err := AdjustTimePeriod(from, to)
	if err != nil {
		return nil, err
	}

	// Get today's date in day-only format
	now := time.Now()
	_, today, _ := AdjustTimePeriod(now, now)

	lbs, err := r.Backend.LoadBalancers()
	if err != nil {
		r.Logger.WithFields(logger.Fields{"from": from, "to": to, "error": err}).Warn("Error getting endpoint/loadbalancers applications processing AllLoadBalancerRelays request")
		return nil, err
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	rawResp := make(map[string]LoadBalancerRelaysResponse)

	for day, counts := range r.dailyUsage {
		for _, lb := range lbs {
			total := rawResp[lb.ID].Count

			var apps []string
			for _, app := range lb.Applications {
				key := applicationPublicKey(app)
				if key != "" {
					apps = append(apps, key)
				}
			}

			// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
			if (day.After(from) || day.Equal(from)) && day.Before(to) {
				for _, app := range apps {
					total.Success += counts[app].Success
					total.Failure += counts[app].Failure
				}
			}

			rawResp[lb.ID] = LoadBalancerRelaysResponse{
				Endpoint:     lb.ID,
				From:         from,
				To:           to,
				Count:        total,
				Applications: apps,
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, lb := range lbs {
			total := rawResp[lb.ID].Count

			var apps []string
			for _, app := range lb.Applications {
				key := applicationPublicKey(app)
				if key != "" {
					apps = append(apps, key)
				}
			}

			for _, app := range apps {
				total.Success += r.todaysUsage[app].Success
				total.Failure += r.todaysUsage[app].Failure
			}

			rawResp[lb.ID] = LoadBalancerRelaysResponse{
				Endpoint:     lb.ID,
				From:         from,
				To:           to,
				Count:        total,
				Applications: apps,
			}
		}
	}

	resp := []LoadBalancerRelaysResponse{}

	for _, relResp := range rawResp {
		resp = append(resp, relResp)
	}

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

func applicationPublicKey(app *repository.Application) string {
	if app == nil {
		return ""
	}
	appKey := app.GatewayAAT.ApplicationPublicKey
	if appKey != "" {
		return appKey
	}
	return ""
}
