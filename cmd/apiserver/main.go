package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/adshmh/meter/api"
	"github.com/adshmh/meter/cmd"
	"github.com/adshmh/meter/db"
)

const (
	SERVER_PORT_DEFAULT = 9898
	ENV_SERVER_PORT     = "API_SERVER_PORT"
)

func getPort(key string, defaultPort int) (int, error) {
	portStr := os.Getenv(key)
	if portStr == "" {
		return defaultPort, nil
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}

	if port == 0 {
		return defaultPort, nil
	}
	return port, nil
}

// TODO: need a /health endpoint
func main() {
	log := logger.New()

	port, err := getPort(ENV_SERVER_PORT, SERVER_PORT_DEFAULT)
	if err != nil {
		log.WithFields(logger.Fields{"error": err, "port": port}).Warn("Invalid port specified")
		os.Exit(1)
	}

	postgresOptions := cmd.GatherPostgresOptions()
	pgClient, err := db.NewPostgresClient(postgresOptions)
	if err != nil {
		fmt.Errorf("Error setting up Postgres client: %v\n", err)
		os.Exit(1)
	}

	// TODO: make the data loader run interval configurable
	meter := api.NewRelayMeter(pgClient, log, 30*time.Second)
	http.HandleFunc("/", api.GetHttpServer(meter, log))

	log.Info("Starting the apiserver...")
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)

	log.Warn("Unexpected exit.")
}
