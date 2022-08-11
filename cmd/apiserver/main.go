package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/pokt-foundation/portal-api-go/repository"

	// TODO: replace with pokt-foundation/relay-meter
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
	ENV_BACKEND_API_TOKEN          = "BACKEND_API_TOKEN"
)

type options struct {
	loadInterval            int
	dailyMetricsTTLSeconds  int
	todaysMetricsTTLSeconds int
	maxPastDays             int
	port                    int
	backendApiUrl           string
	backendApiToken         string
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

	token := os.Getenv(ENV_BACKEND_API_TOKEN)
	if token == "" {
		return options, fmt.Errorf("Missing required environment variable: %s", ENV_BACKEND_API_TOKEN)
	}
	options.backendApiToken = token

	return options, nil
}

type backendProvider struct {
	db.PostgresClient
	backendApiUrl   string
	backendApiToken string
}

func (b *backendProvider) UserApps(user string) ([]string, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/user/%s/application", b.backendApiUrl, user), nil)
	req.Header.Add("Authorization", b.backendApiToken)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Error from backend apiserver: %d, %s", resp.StatusCode, string(body))
	}

	var userApps []repository.Application
	if err := json.Unmarshal(body, &userApps); err != nil {
		return nil, err
	}

	var applications []string
	for _, app := range userApps {
		if app.FreeTier {
			if app.FreeTierAAT.ApplicationPublicKey != "" {
				applications = append(applications, app.FreeTierAAT.ApplicationPublicKey)
			}
		} else {
			if app.GatewayAAT.ApplicationPublicKey != "" {
				applications = append(applications, app.GatewayAAT.ApplicationPublicKey)
			}
		}
	}
	return applications, nil
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
		PostgresClient:  pgClient,
		backendApiUrl:   options.backendApiUrl,
		backendApiToken: options.backendApiToken,
	}
	meter := api.NewRelayMeter(&backend, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(meter, log))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
