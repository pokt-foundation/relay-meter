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
	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/db"
)

var (
	postgresUser     = environment.MustGetString("POSTGRES_USER")
	postgresPassword = environment.MustGetString("POSTGRES_PASSWORD")
	postgresDB       = environment.MustGetString("POSTGRES_DB")
	postgresHost     = environment.MustGetString("POSTGRES_HOST")

	backendAPIURL   = environment.MustGetString("BACKEND_API_URL")
	backendAPIToken = environment.MustGetString("BACKEND_API_TOKEN")

	loadInterval            = int(environment.GetInt64("LOAD_INTERVAL_SECONDS", 30))
	dailyMetricsTTLSeconds  = int(environment.GetInt64("DAILY_METRICS_TTL_SECONDS", 120))
	todaysMetricsTTLSeconds = int(environment.GetInt64("TODAYS_METRICS_TTL_SECONDS", 60))
	maxPastDays             = int(environment.GetInt64("MAX_ARCHIVE_AGE", 30))
	port                    = int(environment.GetInt64("API_SERVER_PORT", 9898))
)

type backendProvider struct {
	db.PostgresClient
	backendAPIURL   string
	backendAPIToken string
}

func (b *backendProvider) UserApps(user string) ([]string, error) {
	// TODO: make the timeout configurable
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/user/%s/application", b.backendAPIURL, user), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.backendAPIToken)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer/%s", b.backendAPIURL, endpoint), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.backendAPIToken)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/load_balancer", b.backendAPIURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", b.backendAPIToken)

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

	// TODO: make the data loader run interval configurable
	meterOptions := api.RelayMeterOptions{
		LoadInterval:     time.Duration(loadInterval) * time.Second,
		DailyMetricsTTL:  time.Duration(dailyMetricsTTLSeconds) * time.Second,
		TodaysMetricsTTL: time.Duration(todaysMetricsTTLSeconds) * time.Second,
		MaxPastDays:      time.Duration(maxPastDays) * 24 * time.Hour,
	}

	backend := backendProvider{
		PostgresClient:  pgClient,
		backendAPIURL:   backendAPIURL,
		backendAPIToken: backendAPIToken,
	}
	meter := api.NewRelayMeter(&backend, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(meter, log))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)

	log.Warn("Unexpected exit.")
}
