//go:build tests

package tests

import (
	"testing"

	"github.com/pokt-foundation/relay-meter/api"
	"github.com/stretchr/testify/suite"
)

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

	ts.True(allZero)
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

func (ts *RelayMeterFunctionalTestSuite) Test_OriginClassification() {
	allOriginsRelays, err := get[[]api.OriginClassificationsResponse](ts.options.relayMeterURL, "v1/relays/origin-classification", "", "", ts.options.relayMeterAPIKey, ts.httpClient)
	ts.NoError(err)

	allZero := true

	for _, result := range allOriginsRelays {
		if (result.Count.Failure + result.Count.Success) > 0 {
			allZero = false
			break
		}
	}

	ts.False(allZero)
}
