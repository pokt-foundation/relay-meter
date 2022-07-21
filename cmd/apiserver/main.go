package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/cmd"
	"github.com/adshmh/meter/db"
)

const (
	LOAD_INTERVAL_DEFAULT_SECONDS      = 30
	DAILY_METRICS_TTL_DEFAULT_SECONDS  = 36000
	TODAYS_METRICS_TTL_DEFAULT_SECONDS = 300
	MAX_ARCHIVE_AGE_DEFAULT_DAYS       = 30
	SERVER_PORT_DEFAULT                = 9898

	ENV_LOAD_INTERVAL_SECONDS      = "LOAD_INTERVAL_SECONDS"
	ENV_DAILY_METRICS_TTL_SECONDS  = "DAILY_METRICS_TTL_SECONDS"
	ENV_TODAYS_METRICS_TTL_SECONDS = "TODAYS_METRICS_TTL_SECONDS"
	ENV_MAX_ARCHIVE_AGE_DAYS       = "MAX_ARCHIVE_AGE"
	ENV_SERVER_PORT                = "API_SERVER_PORT"
	ENV_BACKEND_API_URL            = "BACKEND_API_URL"
)

type options struct {
	loadInterval            int
	dailyMetricsTTLSeconds  int
	todaysMetricsTTLSeconds int
	maxPastDays             int
	port                    int
	backendApiUrl           string
}

func gatherOptions() (options, error) {
	options := options{}

	optsItems := []struct {
		value        *int
		defaultValue int
		envVar       string
	}{
		{value: &options.loadInterval, defaultValue: LOAD_INTERVAL_DEFAULT_SECONDS, envVar: ENV_LOAD_INTERVAL_SECONDS},
		{value: &options.dailyMetricsTTLSeconds, defaultValue: DAILY_METRICS_TTL_DEFAULT_SECONDS, envVar: ENV_DAILY_METRICS_TTL_SECONDS},
		{value: &options.todaysMetricsTTLSeconds, defaultValue: TODAYS_METRICS_TTL_DEFAULT_SECONDS, envVar: ENV_TODAYS_METRICS_TTL_SECONDS},
		{value: &options.maxPastDays, defaultValue: MAX_ARCHIVE_AGE_DEFAULT_DAYS, envVar: ENV_MAX_ARCHIVE_AGE_DAYS},
		{value: &options.port, defaultValue: SERVER_PORT_DEFAULT, envVar: ENV_SERVER_PORT},
	}

	for _, o := range optsItems {
		value, err := cmd.GetIntFromEnv(o.envVar, o.defaultValue)
		if err != nil {
			return options, err
		}
		*o.value = value
	}

	backendUrl := os.Getenv(ENV_BACKEND_API_URL)
	if backendUrl == "" {
		return options, fmt.Errorf("Missing required environment variable: %s", ENV_BACKEND_API_URL)
	}
	options.backendApiUrl = backendUrl
	return options, nil
}

type backendProvider struct {
	db.PostgresClient
	backendApiUrl string
}

func (b *backendProvider) UserApps(user string) ([]string, error) {
	// TODO: add a timeout
	v := url.Values{}
	v.Add("USER", user)
	resp, err := http.Get(fmt.Sprintf("%s/users?%s", b.backendApiUrl, v.Encode()))
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userApps struct {
		User         string
		Applications []string
	}

	if err := json.Unmarshal(body, &userApps); err != nil {
		return nil, err
	}
	return userApps.Applications, nil
}

// TODO: need a /health endpoint
func main() {
	log := logger.New()

	options, err := gatherOptions()
	if err != nil {
		log.WithFields(logger.Fields{"error": err, "options": options}).Warn("Invalid options specified")
		os.Exit(1)
	}

	postgresOptions := cmd.GatherPostgresOptions()
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Errorf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	// TODO: make the data loader run interval configurable
	meterOptions := api.RelayMeterOptions{
		LoadInterval:     time.Duration(options.loadInterval) * time.Second,
		DailyMetricsTTL:  time.Duration(options.dailyMetricsTTLSeconds) * time.Second,
		TodaysMetricsTTL: time.Duration(options.todaysMetricsTTLSeconds) * time.Second,
		MaxPastDays:      time.Duration(options.maxPastDays) * 24 * time.Hour,
	}
	log.WithFields(logger.Fields{"postgresOptions": postgresOptions, "meterOptions": meterOptions}).Info("Gathered options.")

	backend := backendProvider{
		PostgresClient: pgClient,
		backendApiUrl:  options.backendApiUrl,
	}
	meter := api.NewRelayMeter(&backend, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(meter, log))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
