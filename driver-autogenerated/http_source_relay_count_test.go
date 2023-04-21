package postgresdriver

import (
	"context"
	"time"
)

func (ts *PGDriverTestSuite) TestPostgresDriver_HTTPSourceRelayCount() {
	tests := []struct {
		name  string
		count HttpSourceRelayCount
		times int64
		day   time.Time
		err   error
	}{
		{
			name: "Success",
			count: HttpSourceRelayCount{
				AppPublicKey: "2585504a028b138b4b535d2351bc45260a3de9cd66305a854049d1a5143392a8", // pragma: allowlist secret
				Day:          time.Now(),
				Success:      3,
				Error:        3,
			},
			times: 2,
			day:   time.Now(),
			err:   nil,
		},
	}
	for _, tt := range tests {
		for i := 0; i < int(tt.times); i++ {
			ts.Equal(ts.driver.WriteHTTPSourceRelayCount(context.Background(), tt.count), tt.err)
		}

		counts, err := ts.driver.ReadHTTPSourceRelayCounts(context.Background(), tt.count.Day, tt.count.Day)
		ts.Equal(err, tt.err)

		ts.Equal(counts[0].AppPublicKey, tt.count.AppPublicKey)
		ts.Equal(counts[0].Success, tt.count.Success*tt.times)
		ts.Equal(counts[0].Error, tt.count.Error*tt.times)
	}
}
