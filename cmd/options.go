package cmd

import (
	"os"

	"github.com/adshmh/meter/db"
)

const (
	INFLUXDB_URL = "INFLUXDB_URL"
	INFLUXDB_TOKEN = "INFLUXDB_TOKEN"
	INFLUXDB_ORG = "INFLUXDB_ORG"
	INFLUXDB_BUCKET_DAILY = "INFLUXDB_BUCKET_DAILY"
	INFLUXDB_BUCKET_CURRENT = "INFLUXDB_BUCKET_CURRENT"

	POSTGRES_USER = "POSTGRES_USER"
	POSTGRES_PASSWORD = "POSTGRES_PASSWORD"
	POSTGRES_DB = "POSTGRES_DB"
	POSTGRES_HOST = "POSTGRES_HOST"
)

type options struct {
	db.InfluxDBOptions
	db.PostgresOptions
}

func GatherInfluxOptions() db.InfluxDBOptions {
	return db.InfluxDBOptions {
		URL: os.Getenv(INFLUXDB_URL),
		Token: os.Getenv(INFLUXDB_TOKEN),
		Org: os.Getenv(INFLUXDB_ORG),
		DailyBucket: os.Getenv(INFLUXDB_BUCKET_DAILY),
		CurrentBucket: os.Getenv(INFLUXDB_BUCKET_CURRENT),
	}
}

func GatherPostgresOptions() db.PostgresOptions {
	return db.PostgresOptions {
		User: os.Getenv(POSTGRES_USER),
		Password: os.Getenv(POSTGRES_PASSWORD),
		Host: os.Getenv(POSTGRES_HOST),
		DB: os.Getenv(POSTGRES_DB),
	}
}
