package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	phdClient "github.com/pokt-foundation/db-client/client"
	"github.com/pokt-foundation/portal-db/types"
	"github.com/pokt-foundation/utils-go/environment"

	// TODO: replace with pokt-foundation/relay-meter
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/cmd"
	"github.com/pokt-foundation/relay-meter/db"
)

const (
	RELAY_METER_API_KEYS = "API_KEYS"
	PHD_BASE_URL         = "BACKEND_API_URL"
	PHD_API_KEY          = "BACKEND_API_TOKEN"

	LOAD_INTERVAL_SECONDS      = "LOAD_INTERVAL_SECONDS"
	DAILY_METRICS_TTL_SECONDS  = "DAILY_METRICS_TTL_SECONDS"
	TODAYS_METRICS_TTL_SECONDS = "TODAYS_METRICS_TTL_SECONDS"
	MAX_ARCHIVE_AGE            = "MAX_ARCHIVE_AGE"
	API_SERVER_PORT            = "API_SERVER_PORT"
	HTTP_TIMEOUT               = "HTTP_TIMEOUT"
	HTTP_RETRIES               = "HTTP_RETRIES"

	defaultLoadIntervalSeconds      = 30
	defaultDailyMetricsTTLSeconds   = 120
	defaultsTodaysMetricsTTLSeconds = 60
	defaultMaxArchiveAgeDays        = 30
	defaultServerPort               = 9898
	defaultHTTPTimeoutSeconds       = 5
	defaultHTTPRetries              = 0
)

type options struct {
	relayMeterAPIKeys map[string]bool
	phdBaseURL        string
	phdAPIKey         string

	loadInterval            int
	dailyMetricsTTLSeconds  int
	todaysMetricsTTLSeconds int
	maxPastDays             int
	timeout                 time.Duration
	retries                 int
	port                    int
}

func gatherOptions() options {
	return options{
		relayMeterAPIKeys: environment.MustGetStringMap(RELAY_METER_API_KEYS, ";"),
		phdBaseURL:        environment.MustGetString(PHD_BASE_URL),
		phdAPIKey:         environment.MustGetString(PHD_API_KEY),

		loadInterval:            int(environment.GetInt64(LOAD_INTERVAL_SECONDS, defaultLoadIntervalSeconds)),
		dailyMetricsTTLSeconds:  int(environment.GetInt64(DAILY_METRICS_TTL_SECONDS, defaultDailyMetricsTTLSeconds)),
		todaysMetricsTTLSeconds: int(environment.GetInt64(TODAYS_METRICS_TTL_SECONDS, defaultsTodaysMetricsTTLSeconds)),
		maxPastDays:             int(environment.GetInt64(MAX_ARCHIVE_AGE, defaultMaxArchiveAgeDays)),
		timeout:                 time.Duration(environment.GetInt64(HTTP_TIMEOUT, defaultHTTPTimeoutSeconds)) * time.Second,
		retries:                 int(environment.GetInt64(HTTP_RETRIES, defaultHTTPRetries)),
		port:                    int(environment.GetInt64(API_SERVER_PORT, defaultServerPort)),
	}
}

type backendProvider struct {
	db.PostgresClient
	phd phdClient.IDBReader
}

func (p *backendProvider) UserApps(ctx context.Context, user string) ([]string, error) {
	userApps, err := p.phd.GetApplicationsByUserID(ctx, user)
	if err != nil {
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

func (p *backendProvider) LoadBalancer(ctx context.Context, endpoint string) (*types.LoadBalancer, error) {
	return p.phd.GetLoadBalancerByID(ctx, endpoint)
}

func (p *backendProvider) LoadBalancers(ctx context.Context) ([]*types.LoadBalancer, error) {
	return p.phd.GetLoadBalancers(ctx)
}

// TODO: need a /health endpoint
func main() {
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	options := gatherOptions()
	postgresOptions := cmd.GatherPostgresOptions()

	ctx := context.Background()

	// TODO: make the data loader run interval configurable
	meterOptions := api.RelayMeterOptions{
		LoadInterval:     time.Duration(options.loadInterval) * time.Second,
		DailyMetricsTTL:  time.Duration(options.dailyMetricsTTLSeconds) * time.Second,
		TodaysMetricsTTL: time.Duration(options.todaysMetricsTTLSeconds) * time.Second,
		MaxPastDays:      time.Duration(options.maxPastDays) * 24 * time.Hour,
	}
	log.WithFields(logger.Fields{"postgresOptions": postgresOptions, "meterOptions": meterOptions}).Info("Gathered options.")

	/* Init Postgres Client */
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Printf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	/* Init PHD Client */
	phdClient, err := phdClient.NewReadOnlyDBClient(phdClient.Config{
		BaseURL: options.phdBaseURL,
		APIKey:  options.phdAPIKey,
		Version: phdClient.V1,
		Retries: options.retries,
		Timeout: options.timeout,
	})
	if err != nil {
		log.Error(fmt.Sprintf("create PHD client failed with error: %s", err.Error()))
		panic(err)
	}

	backend := &backendProvider{PostgresClient: pgClient, phd: phdClient}

	meter := api.NewRelayMeter(ctx, backend, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(ctx, meter, log, options.relayMeterAPIKeys))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
