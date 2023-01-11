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
	"github.com/pokt-foundation/utils-go/environment"

	// TODO: replace with pokt-foundation/relay-meter
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/cmd"
	"github.com/pokt-foundation/relay-meter/db"
)

const (
	// PHD API
	ENV_BACKEND_API_URL            = "BACKEND_API_URL"
	ENV_BACKEND_API_TOKEN          = "BACKEND_API_TOKEN"
	ENV_LOAD_INTERVAL_SECONDS      = "LOAD_INTERVAL_SECONDS"
	ENV_DAILY_METRICS_TTL_SECONDS  = "DAILY_METRICS_TTL_SECONDS"
	ENV_TODAYS_METRICS_TTL_SECONDS = "TODAYS_METRICS_TTL_SECONDS"
	ENV_MAX_ARCHIVE_AGE_DAYS       = "MAX_ARCHIVE_AGE"
	ENV_SERVER_PORT                = "API_SERVER_PORT"

	LOAD_INTERVAL_DEFAULT_SECONDS      = 30
	DAILY_METRICS_TTL_DEFAULT_SECONDS  = 120
	TODAYS_METRICS_TTL_DEFAULT_SECONDS = 60
	MAX_ARCHIVE_AGE_DEFAULT_DAYS       = 30
	SERVER_PORT_DEFAULT                = 9898
)

type options struct {
	backendApiUrl           string
	backendApiToken         string
	loadInterval            int
	dailyMetricsTTLSeconds  int
	todaysMetricsTTLSeconds int
	maxPastDays             int
	port                    int
	apiKeys                 map[string]bool
}

func gatherOptions() options {
	return options{
		backendApiUrl:           environment.MustGetString(ENV_BACKEND_API_URL),
		backendApiToken:         environment.MustGetString(ENV_BACKEND_API_TOKEN),
		loadInterval:            int(environment.GetInt64(ENV_LOAD_INTERVAL_SECONDS, LOAD_INTERVAL_DEFAULT_SECONDS)),
		dailyMetricsTTLSeconds:  int(environment.GetInt64(ENV_DAILY_METRICS_TTL_SECONDS, DAILY_METRICS_TTL_DEFAULT_SECONDS)),
		todaysMetricsTTLSeconds: int(environment.GetInt64(ENV_TODAYS_METRICS_TTL_SECONDS, TODAYS_METRICS_TTL_DEFAULT_SECONDS)),
		maxPastDays:             int(environment.GetInt64(ENV_MAX_ARCHIVE_AGE_DAYS, MAX_ARCHIVE_AGE_DEFAULT_DAYS)),
		port:                    int(environment.GetInt64(ENV_SERVER_PORT, SERVER_PORT_DEFAULT)),
		apiKeys:                 environment.MustGetStringMap("API_KEYS", ","),
	}
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
	if err != nil {
		return nil, err
	}
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
		if app.GatewayAAT.ApplicationPublicKey != "" {
			applications = append(applications, app.GatewayAAT.ApplicationPublicKey)
		}
	}
	return applications, nil
}

func (b *backendProvider) LoadBalancer(endpoint string) (*repository.LoadBalancer, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer/%s", b.backendApiUrl, endpoint), nil)
	if err != nil {
		return nil, err
	}
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

	var lb repository.LoadBalancer
	if err := json.Unmarshal(body, &lb); err != nil {
		return nil, err
	}
	return &lb, nil
}

func (b *backendProvider) LoadBalancers() ([]*repository.LoadBalancer, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer", b.backendApiUrl), nil)
	if err != nil {
		return nil, err
	}
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

	var lbs []*repository.LoadBalancer
	if err := json.Unmarshal(body, &lbs); err != nil {
		return nil, err
	}
	return lbs, nil
}

// TODO: need a /health endpoint
func main() {
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	options := gatherOptions()

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
	http.HandleFunc("/", api.GetHttpServer(meter, log, options.apiKeys))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
