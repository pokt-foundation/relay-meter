package cmd

import (
	"github.com/adshmh/meter/db"
	"github.com/pokt-foundation/utils-go/environment"
)

const (
	INFLUXDB_URL            = "INFLUXDB_URL"
	INFLUXDB_TOKEN          = "INFLUXDB_TOKEN"
	INFLUXDB_ORG            = "INFLUXDB_ORG"
	INFLUXDB_BUCKET_DAILY   = "INFLUXDB_BUCKET_DAILY"
	INFLUXDB_BUCKET_CURRENT = "INFLUXDB_BUCKET_CURRENT"

	POSTGRES_USER     = "POSTGRES_USER"
	POSTGRES_PASSWORD = "POSTGRES_PASSWORD"
	POSTGRES_HOST     = "POSTGRES_HOST"
	POSTGRES_DB       = "POSTGRES_DB"
)

type options struct {
	db.InfluxDBOptions
	db.PostgresOptions
}

func GatherInfluxOptions() db.InfluxDBOptions {
	return db.InfluxDBOptions{
		URL:           environment.MustGetString(INFLUXDB_URL),
		Token:         environment.MustGetString(INFLUXDB_TOKEN),
		Org:           environment.MustGetString(INFLUXDB_ORG),
		DailyBucket:   environment.MustGetString(INFLUXDB_BUCKET_DAILY),
		CurrentBucket: environment.MustGetString(INFLUXDB_BUCKET_CURRENT),
	}
}

func GatherPostgresOptions() db.PostgresOptions {
	return db.PostgresOptions{
		User:     environment.MustGetString(POSTGRES_USER),
		Password: environment.MustGetString(POSTGRES_PASSWORD),
		Host:     environment.MustGetString(POSTGRES_HOST),
		DB:       environment.MustGetString(POSTGRES_DB),
	}
}
