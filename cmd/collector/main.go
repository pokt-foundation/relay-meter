package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	driver "github.com/pokt-foundation/relay-meter/driver-autogenerated"
	"github.com/pokt-foundation/utils-go/environment"
	logger "github.com/sirupsen/logrus"

	"github.com/pokt-foundation/relay-meter/cmd"
	"github.com/pokt-foundation/relay-meter/collector"
	"github.com/pokt-foundation/relay-meter/db"
)

const (
	collectingIntervalSeconds = "COLLECTION_INTERVAL_SECONDS"
	reportIntervalSeconds     = "REPORT_INTERVAL_SECONDS"
	maxArchiveAgeDays         = "MAX_ARCHIVE_AGE"

	// App Env determines type of DB connection.
	appEnv = "APP_ENV"

	defaultCollectIntervalSeconds = 300
	defaultReportIntervalSeconds  = 30
	defaultMaxArchiveAgeDays      = 30
)

type options struct {
	collectionInterval int
	reportingInterval  int
	maxArchiveAge      time.Duration

	appEnv types.AppEnv
}

func gatherOptions() options {
	appEnv := types.AppEnv(environment.MustGetString(appEnv))
	if !appEnv.IsValid() {
		panic(fmt.Sprintf("invalid APP_ENV: %s", appEnv))
	}

	return options{
		collectionInterval: int(environment.GetInt64(collectingIntervalSeconds, defaultCollectIntervalSeconds)),
		reportingInterval:  int(environment.GetInt64(reportIntervalSeconds, defaultReportIntervalSeconds)),
		maxArchiveAge:      time.Duration(environment.GetInt64(maxArchiveAgeDays, defaultMaxArchiveAgeDays)) * 24 * time.Hour,

		appEnv: appEnv,
	}
}

// TODO: add a /health endpoint
func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}()

	options := gatherOptions()
	postgresOptions := cmd.GatherPostgresOptions()

	/* Init Postgres Client */
	var dbInst *sql.DB

	switch options.appEnv {
	case types.AppEnvProduction:
		dbConn, cleanup, err := db.NewDBConnection(postgresOptions)
		if err != nil {
			panic(fmt.Errorf("Error setting up Postgres connection: %v\n", err))
		}
		dbInst = dbConn

		defer func() {
			err := cleanup()
			if err != nil {
				fmt.Printf("Error during cleanup: %v\n", err)
			}
		}()

	default:
		dbConn, err := db.NewTestDBConnection(postgresOptions)
		if err != nil {
			panic(fmt.Errorf("Error setting up test Postgres connection: %v\n", err))
		}
		dbInst = dbConn
	}

	pgClient := db.NewPostgresClientFromDBInstance(dbInst)
	driver := driver.NewPostgresDriverFromDBInstance(dbInst)

	fmt.Printf("Starting the collector...")
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	collector := collector.NewCollector([]collector.Source{driver}, pgClient, options.maxArchiveAge, log)
	collector.Start(context.Background(), options.collectionInterval, options.reportingInterval)
}
