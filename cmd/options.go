package cmd

import (
	"os"
	"strconv"

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

// TODO: flags package makes this a lot easier, but env. variables are better suited for k8s deployments
//	TODO: see if there is a package so we can skip writing env. processing code
func GetIntFromEnv(envVarName string, defaultValue int) (int, error) {
	str := os.Getenv(envVarName)
	if str == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}

	if value == 0 {
		return defaultValue, nil
	}
	return value, nil
}
