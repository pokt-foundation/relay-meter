package main

import (
	"testing"
	"time"

	"github.com/gojektech/heimdall/httpclient"
	"github.com/pokt-foundation/relay-meter/api"
	"github.com/pokt-foundation/utils-go/environment"
	"github.com/stretchr/testify/suite"
)

type RelayMeterFunctionalTestSuite struct {
	suite.Suite
	options                  FunctionalTestClientOptions
	httpClient               *httpclient.Client
	todayDate, yesterdayDate string
}

type FunctionalTestClientOptions struct {
	relayMeterURL, relayMeterAPIKey string
}

func (ts *RelayMeterFunctionalTestSuite) SetupSuite() {
	ts.options = FunctionalTestClientOptions{
		relayMeterURL:    environment.MustGetString("RELAY_METER_URL"),
		relayMeterAPIKey: environment.MustGetString("RELAY_METER_API_KEY"),
	}

	ts.httpClient = httpclient.NewClient(
		httpclient.WithHTTPTimeout(5*time.Second), httpclient.WithRetryCount(0),
	)

	today, _ := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	yesterday, _ := time.Parse("2006-01-02", time.Now().Add(-time.Hour*24).Format("2006-01-02"))

	ts.todayDate = today.Format("2006-01-02T15:04:05Z")
	ts.yesterdayDate = yesterday.Format("2006-01-02T15:04:05Z")
}

func Test_RunSuite_Functional(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping functional test test")
	}

	testSuite := new(RelayMeterFunctionalTestSuite)
	suite.Run(t, testSuite)
}

func (ts *RelayMeterFunctionalTestSuite) Test_TodayAllRelayApps() {
	allAppsRelays, err := get[[]api.AppRelaysResponse](ts.options.relayMeterURL, "v1/relays/apps", "", ts.todayDate, ts.options.relayMeterAPIKey, ts.httpClient)
	ts.NoError(err)

	allZero := true

	for _, result := range allAppsRelays {
		if (result.Count.Failure + result.Count.Success) > 0 {
			allZero = false
			break
		}
	}

	ts.False(allZero)
}

func (ts *RelayMeterFunctionalTestSuite) Test_YesterdayAllRelayApps() {
	allAppsRelays, err := get[[]api.AppRelaysResponse](ts.options.relayMeterURL, "v1/relays/apps", "", ts.yesterdayDate, ts.options.relayMeterAPIKey, ts.httpClient)
	ts.NoError(err)

	allZero := true

	for _, result := range allAppsRelays {
		if (result.Count.Failure + result.Count.Success) > 0 {
			allZero = false
			break
		}
	}

	ts.False(allZero)
}
