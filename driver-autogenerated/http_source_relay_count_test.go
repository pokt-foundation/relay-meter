package postgresdriver

import (
	"context"
	"time"

	"github.com/pokt-foundation/relay-meter/api"
)

func (ts *PGDriverTestSuite) TestPostgresDriver_HTTPSourceRelayCount() {
	tests := []struct {
		name   string
		count  api.HTTPSourceRelayCount
		counts []api.HTTPSourceRelayCount
		times  int64
		day    time.Time
		err    error
	}{
		{
			name: "Success",
			count: api.HTTPSourceRelayCount{
				AppPublicKey: "2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8", // pragma: allowlist secret
				Day:          time.Date(1999, time.July, 21, 0, 0, 0, 0, &time.Location{}),
				Success:      3,
				Error:        3,
			},
			counts: []api.HTTPSourceRelayCount{
				{
					AppPublicKey: "2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a9", // pragma: allowlist secret
					Day:          time.Date(1999, time.July, 22, 0, 0, 0, 0, &time.Location{}),
					Success:      3,
					Error:        3,
				},
				{
					AppPublicKey: "2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a1", // pragma: allowlist secret
					Day:          time.Date(1999, time.July, 22, 0, 0, 0, 0, &time.Location{}),
					Success:      3,
					Error:        3,
				},
			},
			times: 2,
			day:   time.Date(1999, time.July, 21, 0, 0, 0, 0, &time.Location{}),
			err:   nil,
		},
	}
	for _, tt := range tests {
		for i := 0; i < int(tt.times); i++ {
			ts.Equal(ts.driver.WriteHTTPSourceRelayCount(context.Background(), tt.count), tt.err)
		}

		for i := 0; i < int(tt.times); i++ {
			ts.Equal(ts.driver.WriteHTTPSourceRelayCounts(context.Background(), tt.counts), tt.err)
		}

		counts, err := ts.driver.ReadHTTPSourceRelayCounts(context.Background(), tt.count.Day, tt.count.Day)
		ts.Equal(err, tt.err)

		ts.Equal(counts[0].AppPublicKey, tt.count.AppPublicKey)
		ts.Equal(counts[0].Success, tt.count.Success*tt.times)
		ts.Equal(counts[0].Error, tt.count.Error*tt.times)

		counts, err = ts.driver.ReadHTTPSourceRelayCounts(context.Background(), tt.counts[0].Day, tt.counts[0].Day)
		ts.Equal(err, tt.err)

		// need to convert to map to be able to assert the results
		countsMap := make(map[string]api.HTTPSourceRelayCount, len(counts))
		for _, count := range counts {
			countsMap[count.AppPublicKey] = count
		}

		for _, count := range tt.counts {
			ts.Equal(countsMap[count.AppPublicKey].AppPublicKey, count.AppPublicKey)
			ts.Equal(countsMap[count.AppPublicKey].Success, count.Success*tt.times)
			ts.Equal(countsMap[count.AppPublicKey].Error, count.Error*tt.times)
		}
	}
}
