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
	ENV_COLLECT_INTERVAL_SECONDS = "COLLECTION_INTERVAL_SECONDS"
	ENV_REPORT_INTERVAL_SECONDS  = "REPORT_INTERVAL_SECONDS"
	ENV_MAX_ARCHIVE_AGE_DAYS     = "MAX_ARCHIVE_AGE"

	COLLECT_INTERVAL_DEFAULT_SECONDS = 300
	REPORT_INTERVAL_DEFAULT_SECONDS  = 30
	MAX_ARCHIVE_AGE_DEFAULT_DAYS     = 30
)

type options struct {
	collectionInterval int
	reportingInterval  int
	maxArchiveAgeDays  int
}

func gatherOptions() options {
	return options{
		collectionInterval: int(environment.GetInt64(ENV_COLLECT_INTERVAL_SECONDS, COLLECT_INTERVAL_DEFAULT_SECONDS)),
		reportingInterval:  int(environment.GetInt64(ENV_REPORT_INTERVAL_SECONDS, REPORT_INTERVAL_DEFAULT_SECONDS)),
		maxArchiveAgeDays:  int(environment.GetInt64(ENV_MAX_ARCHIVE_AGE_DAYS, MAX_ARCHIVE_AGE_DEFAULT_DAYS)),
	}
}

// TODO: need a /health endpoint
func main() {
	influxOptions := cmd.GatherInfluxOptions()
	postgresOptions := cmd.GatherPostgresOptions()

	influxClient := db.NewInfluxDBSource(influxOptions)
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Errorf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	options := gatherOptions()

	fmt.Printf("Starting the collector...")
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	collector := collector.NewCollector(influxClient, pgClient, time.Duration(options.maxArchiveAgeDays)*24*time.Hour, log)
	collector.Start(context.Background(), options.collectionInterval, options.reportingInterval)
}
