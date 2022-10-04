package main

import (
	"context"
	"fmt"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/cmd"
	"github.com/adshmh/meter/collector"
	"github.com/adshmh/meter/db"
)

const (
	COLLECT_INTERVAL_DEFAULT_SECONDS = 300
	REPORT_INTERVAL_DEFAULT_SECONDS  = 30
	MAX_ARCHIVE_AGE_DEFAULT_DAYS     = 30

	ENV_COLLECT_INTERVAL_SECONDS = "COLLECTION_INTERVAL_SECONDS"
	ENV_REPORT_INTERVAL_SECONDS  = "REPORT_INTERVAL_SECONDS"
	ENV_MAX_ARCHIVE_AGE_DAYS     = "MAX_ARCHIVE_AGE"
)

type options struct {
	collectionInterval int
	reportingInterval  int
	maxArchiveAgeDays  int
}

func gatherOptions() (options, error) {
	collectionInterval, err := cmd.GetIntFromEnv(ENV_COLLECT_INTERVAL_SECONDS, COLLECT_INTERVAL_DEFAULT_SECONDS)
	if err != nil {
		return options{}, err
	}

	reportingInterval, err := cmd.GetIntFromEnv(ENV_REPORT_INTERVAL_SECONDS, REPORT_INTERVAL_DEFAULT_SECONDS)
	if err != nil {
		return options{}, err
	}

	maxArchiveAge, err := cmd.GetIntFromEnv(ENV_MAX_ARCHIVE_AGE_DAYS, MAX_ARCHIVE_AGE_DEFAULT_DAYS)
	if err != nil {
		return options{}, err
	}

	return options{
		collectionInterval: collectionInterval,
		reportingInterval:  reportingInterval,
		maxArchiveAgeDays:  maxArchiveAge,
	}, nil
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

	options, err := gatherOptions()
	if err != nil {
		fmt.Errorf("Error gathering options: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Starting the collector...")
	collector := collector.NewCollector(influxClient, pgClient, time.Duration(options.maxArchiveAgeDays)*24*time.Hour, logger.New())
	collector.Start(context.Background(), options.collectionInterval, options.reportingInterval)
}
