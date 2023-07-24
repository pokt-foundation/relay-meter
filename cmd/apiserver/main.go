package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	phdClient "github.com/pokt-foundation/db-client/v2/client"
	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/utils-go/environment"

	// TODO: replace with pokt-foundation/relay-meter
	_ "net/http/pprof"

	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/relay-meter/cmd"
	"github.com/pokt-foundation/relay-meter/db"
	driver "github.com/pokt-foundation/relay-meter/driver-autogenerated"
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

func (p *backendProvider) UserPortalAppPubKeys(ctx context.Context, userID types.UserID) ([]types.PortalAppPublicKey, error) {
	userPortalApps, err := p.phd.GetPortalAppsByUser(ctx, userID, types.RoleOwner)
	if err != nil {
		return nil, err
	}

	var appPubKeys []types.PortalAppPublicKey
	for _, app := range userPortalApps {
		for _, aat := range app.AATs {
			if aat.PublicKey != "" {
				appPubKeys = append(appPubKeys, aat.PublicKey)
			}
		}
	}

	return appPubKeys, nil
}

func (p *backendProvider) PortalApp(ctx context.Context, portalAppID types.PortalAppID) (*types.PortalApp, error) {
	return p.phd.GetPortalAppByID(ctx, portalAppID)
}

func (p *backendProvider) PortalApps(ctx context.Context) ([]*types.PortalApp, error) {
	return p.phd.GetAllPortalApps(ctx)
}

// TODO: add a /health endpoint
func main() {
	log := logger.New()
	log.Formatter = &logger.JSONFormatter{}

	go func() {
		log.Println("pprof:", http.ListenAndServe("localhost:6060", nil))
	}()

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
	dbInst, cleanup, err := db.NewDBConnection(postgresOptions)
	if err != nil {
		fmt.Printf("Error setting up Postgres connection: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		err := cleanup()
		if err != nil {
			fmt.Printf("Error during cleanup: %v\n", err)
		}
	}()

	pgClient := db.NewPostgresClientFromDBInstance(dbInst)
	driver := driver.NewPostgresDriverFromDBInstance(dbInst)

	/* Init PHD Client */
	phdClient, err := phdClient.NewReadOnlyDBClient(phdClient.Config{
		BaseURL: options.phdBaseURL,
		APIKey:  options.phdAPIKey,
		Retries: options.retries,
		Timeout: options.timeout,
	})
	if err != nil {
		log.Error(fmt.Sprintf("create PHD client failed with error: %s", err.Error()))
		panic(err)
	}

	backend := &backendProvider{PostgresClient: pgClient, phd: phdClient}

	meter := api.NewRelayMeter(ctx, backend, driver, log, meterOptions)
	http.HandleFunc("/", api.GetHttpServer(ctx, meter, log, options.relayMeterAPIKeys))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", options.port), nil)

	log.Warn("Unexpected exit.")
}
