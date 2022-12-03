package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pokt-foundation/utils-go/environment"
	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/cmd"
	"github.com/adshmh/meter/collector"
	"github.com/adshmh/meter/db"
)

const (
	collectingIntervalSeconds = "COLLECTION_INTERVAL_SECONDS"
	reportIntervalSeconds     = "REPORT_INTERVAL_SECONDS"
	maxArchiveAgeDays         = "MAX_ARCHIVE_AGE"

	defaultCollectIntervalSeconds = 300
	defaultReportIntervalSeconds  = 30
	defaultMaxArchiveAgeDays      = 30
)

type options struct {
	collectionInterval int
	reportingInterval  int
	maxArchiveAgeDays  time.Duration
}

func gatherOptions() options {
	return options{
		collectionInterval: int(environment.GetInt64(collectingIntervalSeconds, defaultCollectIntervalSeconds)),
		reportingInterval:  int(environment.GetInt64(reportIntervalSeconds, defaultReportIntervalSeconds)),
		maxArchiveAgeDays:  time.Duration(environment.GetInt64(maxArchiveAgeDays, defaultMaxArchiveAgeDays)) * 24 * time.Hour,
	}
}

// TODO: need a /health endpoint
func main() {
	influxOptions := cmd.GatherInfluxOptions()
	postgresOptions := cmd.GatherPostgresOptions()

	influxClient := db.NewInfluxDBSource(influxOptions)
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Printf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	options := gatherOptions()

	fmt.Printf("Starting the collector...")
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	collector := collector.NewCollector(influxClient, pgClient, options.maxArchiveAgeDays, log)
	collector.Start(context.Background(), options.collectionInterval, options.reportingInterval)
}
