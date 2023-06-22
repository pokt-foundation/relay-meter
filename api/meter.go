package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	logger "github.com/sirupsen/logrus"
)

const (
	dayFormat = "2006-01-02"

	// TODO: make all time-related parameters configurable
	TTL_DAILY_METRICS_DEFAULT_SECONDS  = 900
	TTL_TODAYS_METRICS_DEFAULT_SECONDS = 600

	MAX_PAST_DAYS_METRICS_DEFAULT_DAYS = 30
)

var (
	ErrPortalAppNotFound  = errors.New("portal application not found")
	ErrAppLatencyNotFound = errors.New("app latency not found")
)

type RelayMeter interface {
	// AppRelays returns total number of relays for the app over the specified time period
	UserRelays(ctx context.Context, user types.UserID, from, to time.Time) (UserRelaysResponse, error)
	TotalRelays(ctx context.Context, from, to time.Time) (TotalRelaysResponse, error)

	// PortalAppRelays returns the metrics for a Portal
	PortalAppRelays(ctx context.Context, portalAppID types.PortalAppID, from, to time.Time) (PortalAppRelaysResponse, error)
	AllPortalAppsRelays(ctx context.Context, from, to time.Time) ([]PortalAppRelaysResponse, error)
	AppLatency(ctx context.Context, portalAppID types.PortalAppID) (AppLatencyResponse, error)
	AllAppsLatencies(ctx context.Context) ([]AppLatencyResponse, error)
	AllRelaysOrigin(ctx context.Context, from, to time.Time) ([]OriginClassificationsResponse, error)
	RelaysOrigin(ctx context.Context, origin string, from, to time.Time) (OriginClassificationsResponse, error)

	WriteHTTPSourceRelayCounts(ctx context.Context, counts []HTTPSourceRelayCount) error
}

type RelayCounts struct {
	Success int64
	Failure int64
}

type Latency struct {
	Time    time.Time
	Latency float64
}

type AppLatencyResponse struct {
	DailyLatency []Latency
	From         time.Time
	To           time.Time
	PortalAppID  types.PortalAppID
}

type OriginClassificationsResponse struct {
	Count  RelayCounts
	From   time.Time
	To     time.Time
	Origin string
}

type UserRelaysResponse struct {
	Count        RelayCounts
	From         time.Time
	To           time.Time
	User         types.UserID
	PortalAppIDs []types.PortalAppID
}

type TotalRelaysResponse struct {
	Count RelayCounts
	From  time.Time
	To    time.Time
}

type PortalAppRelaysResponse struct {
	Count       RelayCounts
	From        time.Time
	To          time.Time
	PortalAppID types.PortalAppID
}

type RelayMeterOptions struct {
	LoadInterval     time.Duration
	DailyMetricsTTL  time.Duration
	TodaysMetricsTTL time.Duration
	MaxPastDays      time.Duration
}

type HTTPSourceRelayCount struct {
	PortalAppID types.PortalAppID `json:"portalAppID"`
	Day         time.Time         `json:"day"`
	Success     int64             `json:"success"`
	Error       int64             `json:"error"`
}

type HTTPSourceRelayCountInput struct {
	PortalAppID types.PortalAppID `json:"portalAppID"`
	Success     int64             `json:"success"`
	Error       int64             `json:"error"`
}

type Backend interface {
	// TODO: reverse map keys order, i.e. map[app]-> map[day]RelayCounts, at PG level
	DailyUsage(from, to time.Time) (map[time.Time]map[types.PortalAppID]RelayCounts, error)
	TodaysUsage() (map[types.PortalAppID]RelayCounts, error)
	TodaysLatency() (map[types.PortalAppID][]Latency, error)
	TodaysOriginUsage() (map[string]RelayCounts, error)

	// Is expected to return the list of portal app IDs owned by the user
	UserPortalAppIDs(ctx context.Context, userID types.UserID) ([]types.PortalAppID, error)
	// PortalApp returns the full portal app struct
	PortalApp(ctx context.Context, portalAppID types.PortalAppID) (*types.PortalApp, error)
	PortalApps(ctx context.Context) ([]*types.PortalApp, error)
}

type Driver interface {
	WriteHTTPSourceRelayCounts(ctx context.Context, counts []HTTPSourceRelayCount) error
}

func NewRelayMeter(ctx context.Context, backend Backend, driver Driver, logger *logger.Logger, options RelayMeterOptions) RelayMeter {
	// PG client
	meter := &relayMeter{
		Backend:           backend,
		Driver:            driver,
		Logger:            logger,
		RelayMeterOptions: options,
	}

	go func() { meter.StartDataLoader(ctx) }()

	return meter
}

// TODO: Add Cache
type relayMeter struct {
	Backend
	Driver
	*logger.Logger

	dailyUsage        map[time.Time]map[types.PortalAppID]RelayCounts
	todaysUsage       map[types.PortalAppID]RelayCounts
	todaysOriginUsage map[string]RelayCounts
	todaysLatency     map[types.PortalAppID][]Latency

	dailyTTL  time.Time
	todaysTTL time.Time
	rwMutex   sync.RWMutex

	RelayMeterOptions
}

func (r *relayMeter) isEmpty() bool {
	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	return len(r.dailyUsage) == 0 || len(r.todaysUsage) == 0 || len(r.todaysOriginUsage) == 0 || len(r.todaysLatency) == 0
}

// TODO: for now, today's data gets overwritten every time. If needed add todays metrics in intervals as they occur in the day
func (r *relayMeter) loadData(from, to time.Time) error {
	var updateDaily, updateToday bool

	now := time.Now()
	var dailyUsage map[time.Time]map[types.PortalAppID]RelayCounts
	var todaysUsage map[types.PortalAppID]RelayCounts
	var todaysOriginUsage map[string]RelayCounts
	var todaysLatency map[types.PortalAppID][]Latency
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

		todaysOriginUsage, err = r.Backend.TodaysOriginUsage()
		if err != nil {
			r.Logger.WithFields(logger.Fields{"error": err}).Warn("Error loading todays origin usage data")
			return err
		}
		r.Logger.WithFields(logger.Fields{"todays_origin_metrics_count": len(todaysOriginUsage)}).Info("Received todays metrics")
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
		r.todaysOriginUsage = todaysOriginUsage
		r.todaysLatency = todaysLatency

		d := r.RelayMeterOptions.TodaysMetricsTTL
		if int(d.Seconds()) == 0 {
			d = time.Duration(TTL_TODAYS_METRICS_DEFAULT_SECONDS) * time.Second
		}

		r.todaysTTL = time.Now().Add(d)
	}

	return nil
}

func (r *relayMeter) AppLatency(ctx context.Context, portalAppID types.PortalAppID) (AppLatencyResponse, error) {
	r.Logger.WithFields(logger.Fields{"portalAppID": portalAppID}).Info("apiserver: Received AppLatency request")

	appLatency := r.todaysLatency[portalAppID]

	if len(appLatency) == 0 {
		return AppLatencyResponse{}, ErrAppLatencyNotFound
	}

	sort.Slice(appLatency, func(i, j int) bool {
		return appLatency[i].Time.Before(appLatency[j].Time)
	})

	return AppLatencyResponse{
		PortalAppID:  portalAppID,
		DailyLatency: appLatency,
		From:         appLatency[0].Time,
		To:           appLatency[len(appLatency)-1].Time,
	}, nil
}

func (r *relayMeter) AllAppsLatencies(ctx context.Context) ([]AppLatencyResponse, error) {
	r.Logger.Info("apiserver: Received AllAppsLatencies request")

	resp := []AppLatencyResponse{}

	for portalAppID, appLatency := range r.todaysLatency {
		if len(appLatency) > 0 {
			sort.Slice(appLatency, func(i, j int) bool {
				return appLatency[i].Time.Before(appLatency[j].Time)
			})

			latencyResp := AppLatencyResponse{
				PortalAppID:  portalAppID,
				DailyLatency: appLatency,
				From:         appLatency[0].Time,
				To:           appLatency[len(appLatency)-1].Time,
			}

			resp = append(resp, latencyResp)
		}
	}

	return resp, nil
}

func (r *relayMeter) AllRelaysOrigin(ctx context.Context, from, to time.Time) ([]OriginClassificationsResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received classifications by origin request")

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

	rawResp := map[string]OriginClassificationsResponse{}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for origin, count := range r.todaysOriginUsage {
			rawResp[origin] = OriginClassificationsResponse{
				Origin: origin,
				Count:  count,
				From:   from,
				To:     to,
			}
		}
	}

	resp := []OriginClassificationsResponse{}

	for _, relResp := range rawResp {
		resp = append(resp, relResp)
	}

	return resp, nil
}

func (r *relayMeter) RelaysOrigin(ctx context.Context, origin string, from, to time.Time) (OriginClassificationsResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received classifications by origin request")

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, to, err := AdjustTimePeriod(from, to)
	if err != nil {
		return OriginClassificationsResponse{}, err
	}

	// Get today's date in day-only format
	now := time.Now()
	_, today, _ := AdjustTimePeriod(now, now)

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	resp := OriginClassificationsResponse{}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for curentOrigin, count := range r.todaysOriginUsage {
			if strings.Contains(curentOrigin, origin) {
				resp = OriginClassificationsResponse{
					Origin: origin,
					Count:  count,
					To:     to,
					From:   from,
				}
				break
			}
		}
	}

	return resp, nil
}

// TODO: refactor the common processing done by both AppRelays and UserRelays
func (r *relayMeter) UserRelays(ctx context.Context, user types.UserID, from, to time.Time) (UserRelaysResponse, error) {
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

	portalAppIDs, err := r.Backend.UserPortalAppIDs(ctx, user)
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
			for _, app := range portalAppIDs {
				total.Success += counts[app].Success
				total.Failure += counts[app].Failure
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, portalAppID := range portalAppIDs {
			total.Success += r.todaysUsage[portalAppID].Success
			total.Failure += r.todaysUsage[portalAppID].Failure
		}
	}

	resp.Count = total
	resp.From = from
	resp.To = to
	resp.PortalAppIDs = portalAppIDs

	return resp, nil
}

func (r *relayMeter) TotalRelays(ctx context.Context, from, to time.Time) (TotalRelaysResponse, error) {
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

// PortalAppRelays returns the metrics of a Portal Application
func (r *relayMeter) PortalAppRelays(ctx context.Context, portalAppID types.PortalAppID, from, to time.Time) (PortalAppRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"portalAppID": portalAppID, "from": from, "to": to}).Info("apiserver: Received PortalAppRelays request")
	resp := PortalAppRelaysResponse{
		From:        from,
		To:          to,
		PortalAppID: portalAppID,
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

	portalApp, err := r.Backend.PortalApp(ctx, portalAppID)
	if err != nil {
		r.Logger.WithFields(logger.Fields{"portalAppID": portalAppID, "from": from, "to": to, "error": err}).Warn("Error getting portal apps processing PortalAppRelays request")
		return resp, err
	}
	if portalApp == nil {
		return resp, ErrPortalAppNotFound
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	var total RelayCounts
	for day, counts := range r.dailyUsage {
		// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
		if (day.After(from) || day.Equal(from)) && day.Before(to) {
			total.Success += counts[portalAppID].Success
			total.Failure += counts[portalAppID].Failure
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		total.Success += r.todaysUsage[portalAppID].Success
		total.Failure += r.todaysUsage[portalAppID].Failure
	}

	resp.Count = total
	resp.From = from
	resp.To = to

	return resp, nil
}

// AllPortalAppsRelays returns the metrics for all portal applications
func (r *relayMeter) AllPortalAppsRelays(ctx context.Context, from, to time.Time) ([]PortalAppRelaysResponse, error) {
	r.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("apiserver: Received AllPortalAppRelays request")

	// TODO: enforce MaxArchiveAge on From parameter
	// TODO: enforce Today as maximum value for To parameter
	from, to, err := AdjustTimePeriod(from, to)
	if err != nil {
		return nil, err
	}

	// Get today's date in day-only format
	now := time.Now()
	_, today, _ := AdjustTimePeriod(now, now)

	portalApps, err := r.Backend.PortalApps(ctx)
	if err != nil {
		r.Logger.WithFields(logger.Fields{"from": from, "to": to, "error": err}).Warn("Error getting portal apps processing AllPortalAppRelays request")
		return nil, err
	}

	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	rawResp := make(map[types.PortalAppID]PortalAppRelaysResponse)

	for day, counts := range r.dailyUsage {
		for _, portalApp := range portalApps {
			total := rawResp[portalApp.ID].Count

			// Note: Equal is not tested for 'to' parameter, as it is already adjusted to the start of the day after the specified date.
			if (day.After(from) || day.Equal(from)) && day.Before(to) {
				total.Success += counts[portalApp.ID].Success
				total.Failure += counts[portalApp.ID].Failure
			}

			rawResp[portalApp.ID] = PortalAppRelaysResponse{
				PortalAppID: portalApp.ID,
				From:        from,
				To:          to,
				Count:       total,
			}
		}
	}

	// TODO: Add a 'Notes' []string field to output: to provide an explanation when the input 'from' or 'to' parameters are corrected.
	if today.Equal(to) || today.Before(to) {
		for _, portalApp := range portalApps {
			total := rawResp[portalApp.ID].Count

			total.Success += r.todaysUsage[portalApp.ID].Success
			total.Failure += r.todaysUsage[portalApp.ID].Failure

			rawResp[portalApp.ID] = PortalAppRelaysResponse{
				PortalAppID: portalApp.ID,
				From:        from,
				To:          to,
				Count:       total,
			}
		}
	}

	resp := []PortalAppRelaysResponse{}

	for _, relResp := range rawResp {
		resp = append(resp, relResp)
	}

	return resp, nil
}

// Starts a data loader in a go routine, to periodically load data from the backend
//
//	context allows stopping the data loader
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
//   - From is adjusted to the start of the day that it originally specifies
//   - To is adjusted to the start of the next day from the day it originally specifies
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
