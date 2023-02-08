//go:build tests

package tests

import (
	"time"

	"github.com/gojektech/heimdall/httpclient"
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
