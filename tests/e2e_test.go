package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adshmh/meter/collector"
	"github.com/adshmh/meter/db"
	logger "github.com/sirupsen/logrus"
)

var (
	influxOptions = db.InfluxDBOptions{
		URL:           "http://localhost:8086",
		Token:         "mytoken",
		Org:           "myorg",
		DailyBucket:   "mainnetRelay",
		CurrentBucket: "mainnetRelay",
	}

	postgresOptions = db.PostgresOptions{
		Host:     "localhost:5432",
		User:     "postgres",
		Password: "pgpassword",
		DB:       "postgres",
	}
)

func Test_E2E_CollectAndWriteTemp(t *testing.T) {
	// c := require.New(t)

	// // docker exec -it influxdb influx write -b mainnetRelay -f ./var/lib/init-data/test-init.csv -o myorg -t mytoken
	// _, err := exec.Command("docker", "exec", "-it", "influxdb", "influx", "write", "-b", "mainnetRelay", "-f", "/var/lib/init-data/test-init.csv", "-o", "myorg", "-t", "mytoken").Output()
	// c.NoError(err)

	influxClient := db.NewInfluxDBSource(influxOptions)
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Println("INIT ERROR", err)

	}

	fmt.Printf("Starting the collector...")
	collector := collector.NewCollector(influxClient, pgClient, time.Duration(30)*24*time.Hour, logger.New())
	collector.Start(context.Background(), 300, 30)
}
