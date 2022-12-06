package collector

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/api"
)

const (
	COLLECT_INTERVAL_SECONDS = 120
	REPORT_INTERVAL_SECONDS  = 10
)

type Source interface {
	DailyCounts(from, to time.Time) (map[time.Time]map[string]api.RelayCounts, error)
	TodaysCounts() (map[string]api.RelayCounts, error)
	TodaysCountsPerOrigin() (map[string]api.RelayCounts, error)
	TodaysLatency() (map[string][]api.Latency, error)
}

type Writer interface {
	// Returns the 2 timestamps which mark the first and last day for
	//	which the metrics are saved.
	//	It is assumed that there are no gaps in the returned time period.
	ExistingMetricsTimespan() (time.Time, time.Time, error)
	// TODO: allow overwriting today's metrics
	WriteTodaysMetrics(counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts, latencies map[string][]api.Latency) error
	WriteDailyUsage(counts map[time.Time]map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error
	WriteTodaysUsage(ctx context.Context, tx *sql.Tx, counts map[string]api.RelayCounts, countsOrigin map[string]api.RelayCounts) error
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
func NewCollector(source Source, writer Writer, maxArchiveAge time.Duration, log *logger.Logger) Collector {
	return &collector{
		Source:        source,
		Writer:        writer,
		MaxArchiveAge: maxArchiveAge,
		Logger:        log,
	}
}

type collector struct {
	Source
	Writer
	MaxArchiveAge time.Duration
	*logger.Logger
}

// Collects relay usage data from the source and uses the writer to store.
//
//	-
func (c *collector) CollectDailyUsage(from, to time.Time) error {
	c.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("Starting daily metrics collection...")
	from, to, err := api.AdjustTimePeriod(from, to)
	if err != nil {
		return err
	}
	c.Logger.WithFields(logger.Fields{"from": from, "to": to}).Info("Daily metrics collection period adjusted.")

	counts, err := c.Source.DailyCounts(from, to)
	if err != nil {
		return err
	}
	c.Logger.WithFields(logger.Fields{"daily_metrics_count": len(counts), "from": from, "to": to}).Info("Collected daily metrics")

	// TODO: Add counts per origins
	return c.Writer.WriteDailyUsage(counts, nil)
}

func (c *collector) collectTodaysUsage() error {
	todaysCounts, err := c.Source.TodaysCounts()
	if err != nil {
		c.Logger.WithFields(logger.Fields{"error": err}).Warn("Failed to collect daily counts")
	}
	c.Logger.WithFields(logger.Fields{"todays_usage_count": len(todaysCounts)}).Info("Collected todays usage")

	todaysRelaysInOrigin, err := c.Source.TodaysCountsPerOrigin()
	if err != nil {
		return err
	}
	c.Logger.WithFields(logger.Fields{"todays_metrics_count_per_origin": len(todaysRelaysInOrigin)}).Info("Collected todays metrics")

	todaysLatency, err := c.Source.TodaysLatency()
	if err != nil {
		c.Logger.WithFields(logger.Fields{"error": err}).Warn("Failed to collect daily latencies")
	}
	c.Logger.WithFields(logger.Fields{"todays_latencies_count": len(todaysLatency)}).Info("Collected todays latencies")

	return c.Writer.WriteTodaysMetrics(todaysCounts, todaysRelaysInOrigin, todaysLatency)
}

func (c *collector) collect() error {
	if err := c.collectTodaysUsage(); err != nil {
		c.Logger.WithFields(logger.Fields{"error": err}).Warn("Failed to write todays metrics")
		return err
	}

	first, last, err := c.Writer.ExistingMetricsTimespan()
	if err != nil {
		return err
	}
	c.Logger.WithFields(logger.Fields{"first": first, "last": last}).Info("Verified existing daily metrics")

	// We assume there are no gaps between stored metrics from start to end, so
	// 	start collecting metrics after the last saved date
	dayLayout := "2006-01-02"
	today, err := time.Parse(dayLayout, time.Now().Format(dayLayout))
	if err != nil {
		return err
	}
	if last.Equal(today.AddDate(0, 0, -1)) || last.After(today.AddDate(0, 0, -1)) {
		c.Logger.WithFields(logger.Fields{"today": today, "last_daily_collected": last}).Info("Last collected daily metric was yesterday, skipping daily metrics collection...")
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
		c.Logger.WithFields(logger.Fields{"error": err}).Warn("Failed to collect data")
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
				c.Logger.WithFields(logger.Fields{"error": err}).Warn("Failed to collect data")
			}
			c.Logger.Info("Data collection completed.")
			remaining = collectIntervalSeconds
		}
	}
}
