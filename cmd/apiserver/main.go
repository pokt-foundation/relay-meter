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

	"github.com/pokt-foundation/portal-db/types"
	"github.com/pokt-foundation/utils-go/environment"

	// TODO: replace with pokt-foundation/relay-meter
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/cmd"
	"github.com/pokt-foundation/relay-meter/db"
)

const (
	relayMeterAPIKeys       = "API_KEYS"
	phdBaseURL              = "BACKEND_API_URL"
	phdAPIKey               = "BACKEND_API_TOKEN"
	loadIntervalSeconds     = "LOAD_INTERVAL_SECONDS"
	failtMetricsTTLSeconds  = "DAILY_METRICS_TTL_SECONDS"
	todaysMetricsTTLSeconds = "TODAYS_METRICS_TTL_SECONDS"
	maxArchiveAgeDays       = "MAX_ARCHIVE_AGE"
	serverPort              = "API_SERVER_PORT"

	defaultLoadIntervalSeconds      = 30
	defaultDailyMetricsTTLSeconds   = 120
	defaultsTodaysMetricsTTLSeconds = 60
	defaultMaxArchiveAgeDays        = 30
	defaultServerPort               = 9898
)

type options struct {
	phdBaseURL              string
	phdAPIKey               string
	loadInterval            int
	dailyMetricsTTLSeconds  int
	todaysMetricsTTLSeconds int
	maxPastDays             int
	port                    int
	relayMeterAPIKeys       map[string]bool
}

func gatherOptions() options {
	return options{
		relayMeterAPIKeys: environment.MustGetStringMap(relayMeterAPIKeys, ";"),
		phdBaseURL:        environment.MustGetString(phdBaseURL),
		phdAPIKey:         environment.MustGetString(phdAPIKey),

		loadInterval:            int(environment.GetInt64(loadIntervalSeconds, defaultLoadIntervalSeconds)),
		dailyMetricsTTLSeconds:  int(environment.GetInt64(failtMetricsTTLSeconds, defaultDailyMetricsTTLSeconds)),
		todaysMetricsTTLSeconds: int(environment.GetInt64(todaysMetricsTTLSeconds, defaultsTodaysMetricsTTLSeconds)),
		maxPastDays:             int(environment.GetInt64(maxArchiveAgeDays, defaultMaxArchiveAgeDays)),
		port:                    int(environment.GetInt64(serverPort, defaultServerPort)),
	}
}

type backendProvider struct {
	db.PostgresClient
	phdBaseURL string
	phdAPIKey  string
}

func (b *backendProvider) UserApps(user string) ([]string, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/user/%s/application", b.phdBaseURL, user), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.phdAPIKey)

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

	var userApps []types.Application
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

func (b *backendProvider) LoadBalancer(endpoint string) (*types.LoadBalancer, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer/%s", b.phdBaseURL, endpoint), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.phdAPIKey)

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

	var lb types.LoadBalancer
	if err := json.Unmarshal(body, &lb); err != nil {
		return nil, err
	}
	return &lb, nil
}

func (b *backendProvider) LoadBalancers() ([]*types.LoadBalancer, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	fmt.Println("HERE", fmt.Sprintf("%s/load_balancer", b.phdBaseURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer", b.phdBaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.phdAPIKey)

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

	var lbs []*types.LoadBalancer
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
		PostgresClient: pgClient,
		phdBaseURL:     options.phdBaseURL,
		phdAPIKey:      options.phdAPIKey,
	}
	meter := api.NewRelayMeter(&backend, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(meter, log, options.relayMeterAPIKeys))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
