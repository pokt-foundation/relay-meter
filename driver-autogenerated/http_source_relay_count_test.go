package postgresdriver

import (
	"context"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
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
				PortalAppID: "test_956d67d3ea93cbfe18a",
				Day:         time.Date(1999, time.July, 21, 0, 0, 0, 0, &time.Location{}),
				Success:     3,
				Error:       3,
			},
			counts: []api.HTTPSourceRelayCount{
				{
					PortalAppID: "test_6b2faf2e3b061651297",
					Day:         time.Date(1999, time.July, 22, 0, 0, 0, 0, &time.Location{}),
					Success:     3,
					Error:       3,
				},
				{
					PortalAppID: "test_d609ae9e66fbcb266f9",
					Day:         time.Date(1999, time.July, 22, 0, 0, 0, 0, &time.Location{}),
					Success:     3,
					Error:       3,
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

		ts.Equal(counts[0].PortalAppID, tt.count.PortalAppID)
		ts.Equal(counts[0].Success, tt.count.Success*tt.times)
		ts.Equal(counts[0].Error, tt.count.Error*tt.times)

		counts, err = ts.driver.ReadHTTPSourceRelayCounts(context.Background(), tt.counts[0].Day, tt.counts[0].Day)
		ts.Equal(err, tt.err)

		// need to convert to map to be able to assert the results
		countsMap := make(map[types.PortalAppID]api.HTTPSourceRelayCount, len(counts))
		for _, count := range counts {
			countsMap[count.PortalAppID] = count
		}

		for _, count := range tt.counts {
			ts.Equal(countsMap[count.PortalAppID].PortalAppID, count.PortalAppID)
			ts.Equal(countsMap[count.PortalAppID].Success, count.Success*tt.times)
			ts.Equal(countsMap[count.PortalAppID].Error, count.Error*tt.times)
		}
	}
}
