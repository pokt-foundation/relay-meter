package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/utils-go/logger"
)

const (
	COLLECT_INTERVAL_SECONDS = 120
	REPORT_INTERVAL_SECONDS  = 10
)

type Source interface {
	DailyCounts(from time.Time, to time.Time) (map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts, error)
	TodaysCounts() (map[types.PortalAppPublicKey]api.RelayCounts, error)
	TodaysCountsPerOrigin() (map[types.PortalAppOrigin]api.RelayCounts, error)
	TodaysLatency() (map[types.PortalAppPublicKey][]api.Latency, error)
	Name() string
}

type Writer interface {
	// Returns the 2 timestamps which mark the first and last day for
	//	which the metrics are saved.
	//	It is assumed that there are no gaps in the returned time period.
	ExistingMetricsTimespan() (time.Time, time.Time, error)
	// TODO: allow overwriting today's metrics
	WriteTodaysMetrics(counts map[types.PortalAppPublicKey]api.RelayCounts, countsOrigin map[types.PortalAppOrigin]api.RelayCounts, latencies map[types.PortalAppPublicKey][]api.Latency) error
	WriteDailyUsage(counts map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts, countsOrigin map[types.PortalAppOrigin]api.RelayCounts) error
	WriteTodaysUsage(ctx context.Context, tx *sql.Tx, counts map[types.PortalAppPublicKey]api.RelayCounts, countsOrigin map[types.PortalAppOrigin]api.RelayCounts) error
}

type Collector interface {
	// Start a goroutine, which collects data at set intervals
	//	The routine respects existing metrics, i.e. will not collect/overwrite existing metrics
	//	expect for today's metrics
	Start(ctx context.Context, collectIntervalSeconds, reportIntervalSeconds int)
	// Collect and write metrics data: this will overwrite any existing metrics
	//	This function exists to allow manually overriding the collector's behavior.
	CollectDailyUsage(from, to time.Time) error
}

// NewCollector returns a collector which will periodically (or on Collect being called)
//
//	gathers metrics from the source and writes to the writer.
//	maxArchiveAge is the oldest time for which metrics are saved
func NewCollector(sources []Source, writer Writer, maxArchiveAge time.Duration, log *logger.Logger) Collector {
	return &collector{
		Sources:       sources,
		Writer:        writer,
		MaxArchiveAge: maxArchiveAge,
		Logger:        log,
	}
}

type collector struct {
	Sources []Source
	Writer
	MaxArchiveAge time.Duration
	*logger.Logger
}

// Collects relay usage data from the source and uses the writer to store.
//
//	-
func (c *collector) CollectDailyUsage(from, to time.Time) error {
	c.Logger.Info("Starting daily metrics collection...",
		slog.Time("from", from),
		slog.Time("to", to),
	)
	from, to, err := api.AdjustTimePeriod(from, to)
	if err != nil {
		return err
	}
	c.Logger.Info("Daily metrics collection period adjusted.",
		slog.Time("from", from),
		slog.Time("to", to),
	)

	var sourcesCounts []map[time.Time]map[types.PortalAppPublicKey]api.RelayCounts

	for _, source := range c.Sources {
		sourceCounts, err := source.DailyCounts(from, to)
		if err != nil {
			return err
		}
		c.Logger.Info("Collected daily metrics",
			slog.Int("daily_metrics_count", len(sourceCounts)),
			slog.String("source", source.Name()),
			slog.Time("from", from),
			slog.Time("to", to),
		)
		sourcesCounts = append(sourcesCounts, sourceCounts)
	}

	counts := mergeTimeRelayCountsMaps(sourcesCounts)

	// TODO: Add counts per origins
	return c.Writer.WriteDailyUsage(counts, nil)

}

func (c *collector) collectTodaysUsage() error {
	var sourcesTodaysCounts []map[types.PortalAppPublicKey]api.RelayCounts
	var sourcesTodaysRelaysInOrigin []map[types.PortalAppOrigin]api.RelayCounts
	var sourcesTodaysLatency []map[types.PortalAppPublicKey][]api.Latency

	for _, source := range c.Sources {
		sourceTodaysCounts, err := source.TodaysCounts()
		if err != nil {
			c.Logger.Warn("Failed to collect daily counts",
				slog.String("error", err.Error()),
			)
		}
		c.Logger.Info("Collected todays usage",
			slog.Int("todays_usage_count", len(sourceTodaysCounts)),
			slog.String("source", source.Name()),
		)
		sourcesTodaysCounts = append(sourcesTodaysCounts, sourceTodaysCounts)

		sourceTodaysRelaysInOrigin, err := source.TodaysCountsPerOrigin()
		if err != nil {
			return err
		}
		c.Logger.Info("Collected todays metrics",
			slog.Int("todays_metrics_count_per_origin", len(sourceTodaysRelaysInOrigin)),
			slog.String("source", source.Name()),
		)
		sourcesTodaysRelaysInOrigin = append(sourcesTodaysRelaysInOrigin, sourceTodaysRelaysInOrigin)

		sourceTodaysLatency, err := source.TodaysLatency()
		if err != nil {
			c.Logger.Warn("Failed to collect daily latencies",
				slog.String("error", err.Error()),
			)
		}
		c.Logger.Info("Collected todays latencies",
			slog.Int("todays_latencies_count", len(sourceTodaysLatency)),
			slog.String("source", source.Name()),
		)
		sourcesTodaysLatency = append(sourcesTodaysLatency, sourceTodaysLatency)
	}

	todaysCounts := mergeRelayCountsMaps(sourcesTodaysCounts)
	todaysRelaysInOrigin := mergeRelayCountsMapsByOrigin(sourcesTodaysRelaysInOrigin)
	todaysLatency := mergeLatencyMaps(sourcesTodaysLatency)

	return c.Writer.WriteTodaysMetrics(todaysCounts, todaysRelaysInOrigin, todaysLatency)
}

func (c *collector) collect() error {
	if err := c.collectTodaysUsage(); err != nil {
		c.Logger.Warn("Failed to write todays metrics",
			slog.String("error", err.Error()),
		)
		return err
	}

	first, last, err := c.Writer.ExistingMetricsTimespan()
	if err != nil {
		return err
	}
	c.Logger.Info("Verified existing daily metrics",
		slog.Time("first", first),
		slog.Time("last", last),
	)

	// We assume there are no gaps between stored metrics from start to end, so
	// 	start collecting metrics after the last saved date
	dayLayout := "2006-01-02"
	today, err := time.Parse(dayLayout, time.Now().Format(dayLayout))
	if err != nil {
		return err
	}
	if last.Equal(today.AddDate(0, 0, -1)) || last.After(today.AddDate(0, 0, -1)) {
		c.Logger.Info("Last collected daily metric was yesterday, skipping daily metrics collection...",
			slog.Time("today", today),
			slog.Time("last_daily_collected", last),
		)
		return nil
	}
	var from time.Time
	if first.Equal(time.Time{}) {
		from = time.Now().Add(-1 * c.MaxArchiveAge)
	} else {
		from = last.AddDate(0, 0, 1)
		if from.After(today) {
			from = today
		}
	}

	// TODO: cover with unit tests
	return c.CollectDailyUsage(from, time.Now().AddDate(0, 0, -1))
}

func (c *collector) Start(ctx context.Context, collectIntervalSeconds, reportIntervalSeconds int) {
	// Do an initial data collection, and then repeat on set intervals
	c.Logger.Info("Starting initial data collection...")
	if err := c.collect(); err != nil {
		c.Logger.Warn("Failed to collect data",
			slog.String("error", err.Error()),
		)
	}
	c.Logger.Info("Initial data collection completed.")

	reportTicker := time.NewTicker(time.Duration(reportIntervalSeconds) * time.Second)
	collectTicker := time.NewTicker(time.Duration(collectIntervalSeconds) * time.Second)

	remaining := collectIntervalSeconds
	for {
		select {
		case <-ctx.Done():
			c.Logger.Warn("Context has been cancelled. Collecter exiting.")
			return
		case <-reportTicker.C:
			remaining -= reportIntervalSeconds
			c.Logger.Info(fmt.Sprintf("Will collect data in %d seconds...", remaining))
		case <-collectTicker.C:
			c.Logger.Info("Starting data collection...")
			if err := c.collect(); err != nil {
				c.Logger.Warn("Failed to collect data",
					slog.String("error", err.Error()),
				)
			}
			c.Logger.Info("Data collection completed.")
			remaining = collectIntervalSeconds
		}
	}
}
