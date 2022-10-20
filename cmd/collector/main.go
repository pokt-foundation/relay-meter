package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pokt-foundation/utils-go/environment"
	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/collector"
	"github.com/adshmh/meter/db"
)

var (
	influxDBURL           = environment.MustGetString("INFLUXDB_URL")
	influxDBToken         = environment.MustGetString("INFLUXDB_TOKEN")
	influxDBOrg           = environment.MustGetString("INFLUXDB_ORG")
	influxDBBucketDaily   = environment.MustGetString("INFLUXDB_BUCKET_DAILY")
	influxDBBucketCurrent = environment.MustGetString("INFLUXDB_BUCKET_CURRENT")

	postgresUser     = environment.MustGetString("POSTGRES_USER")
	postgresPassword = environment.MustGetString("POSTGRES_PASSWORD")
	postgresDB       = environment.MustGetString("POSTGRES_DB")
	postgresHost     = environment.MustGetString("POSTGRES_HOST")

	collectionInterval = int(environment.GetInt64("COLLECTION_INTERVAL_SECONDS", 300))
	reportingInterval  = int(environment.GetInt64("REPORT_INTERVAL_SECONDS", 30))
	maxArchiveAgeDays  = int(environment.GetInt64("MAX_ARCHIVE_AGE", 30))
)

// TODO: need a /health endpoint
func main() {
	influxOptions := db.InfluxDBOptions{
		URL:           influxDBURL,
		Token:         influxDBToken,
		Org:           influxDBOrg,
		DailyBucket:   influxDBBucketDaily,
		CurrentBucket: influxDBBucketCurrent,
	}
	influxClient := db.NewInfluxDBSource(influxOptions)

	postgresOptions := db.PostgresOptions{
		User:     postgresUser,
		Password: postgresPassword,
		Host:     postgresHost,
		DB:       postgresDB,
	}
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Errorf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting the collector...")
	collector := collector.NewCollector(influxClient, pgClient, time.Duration(maxArchiveAgeDays)*24*time.Hour, logger.New())
	collector.Start(context.Background(), collectionInterval, reportingInterval)
}
