package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	logger "github.com/sirupsen/logrus"
)

func TestGetHttpServer(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		url                string
		expected           string
		expectedStatusCode int
		failAuth           bool
	}{
		{
			name: "App relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/apps/app?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "User relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/users/user?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "All apps relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/apps?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Total relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "All load balancers relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/endpoints?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Invalid request path returns an error",
			url:                "http://relay-meter.pokt.network/invalid-path",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Origin usage path is handled correctly",
			url: fmt.Sprint("http://relay-meter.pokt.network/v1/relays/origin-classification",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Failed auhtorization",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/endpoints?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			failAuth:           true,
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpServer := GetHttpServer(context.Background(), &fakeRelayMeter{}, logger.New(), map[string]bool{"dummy": true})

			req := httptest.NewRequest("GET", tc.url, nil)
			if !tc.failAuth {
				req.Header.Add("Authorization", "dummy")
			}
			w := httptest.NewRecorder()

			httpServer(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			if !tc.failAuth {
				body, _ := io.ReadAll(resp.Body)
				var r AppRelaysResponse
				if err := json.Unmarshal(body, &r); err != nil {
					t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
				}
			}
		})
	}

}

func TestHandleAppRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      AppRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: AppRelaysResponse{
				Count:       RelayCounts{Success: 5, Failure: 3},
				From:        now,
				To:          now,
				Application: "app",
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: AppRelaysResponse{
				Count:       RelayCounts{},
				From:        now,
				To:          now,
				Application: "internal meter error",
			},
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "Application not found returns a not found response",
			meterResponse: AppRelaysResponse{
				Count:       RelayCounts{},
				From:        now,
				To:          now,
				Application: "non-existent-app",
			},
			meterErr:           AppNotFound,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Bad request returns reqest error response",
			meterResponse: AppRelaysResponse{
				Count:       RelayCounts{},
				From:        now,
				To:          now,
				Application: "app",
			},
			meterErr:           InvalidRequest,
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMeter := fakeRelayMeter{
				response:    tc.meterResponse,
				responseErr: tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/apps/app?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleAppRelays(context.Background(), &fakeMeter, logger.New(), "app", w, req)

			if !fakeMeter.requestedFrom.Equal(now) {
				t.Fatalf("Expected %v on 'from' parameter, got: %v", now, fakeMeter.requestedFrom)
			}

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			// TODO: Should we return json even if there is a request/internal server error?
			if tc.expectedStatusCode != http.StatusOK {
				return
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: %s, got: %s", "application/json", resp.Header.Get("Content-Type"))
			}

			var r AppRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
			}
			if r.Count != tc.meterResponse.Count {
				t.Errorf("Expected Count: %d, got: %d", tc.meterResponse.Count, r.Count)
			}
		})
	}
}

func TestHandleAllAppsRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      []AppRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: []AppRelaysResponse{
				{
					Application: "app1",
					From:        now.AddDate(0, 0, -30),
					To:          now.AddDate(0, 0, 1),
					Count: RelayCounts{
						Success: 62,
						Failure: 58,
					},
				},
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Error from the meter returns an internal error response",
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:               "Application not found returns a not found response",
			meterErr:           AppNotFound,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "Bad request returns reqest error response",
			meterErr:           InvalidRequest,
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMeter := fakeRelayMeter{
				allResponse: tc.meterResponse,
				responseErr: tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/apps?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleAllAppsRelays(context.Background(), &fakeMeter, logger.New(), w, req)

			if !fakeMeter.requestedFrom.Equal(now) {
				t.Fatalf("Expected %v on 'from' parameter, got: %v", now, fakeMeter.requestedFrom)
			}

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			// TODO: Should we return json even if there is a request/internal server error?
			if tc.expectedStatusCode != http.StatusOK {
				return
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: %s, got: %s", "application/json", resp.Header.Get("Content-Type"))
			}

			var r []AppRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
			}

			if diff := cmp.Diff(tc.meterResponse, r); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHandleLoadBalancerRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      LoadBalancerRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: LoadBalancerRelaysResponse{
				Count:        RelayCounts{Success: 5, Failure: 3},
				From:         now,
				To:           now,
				Endpoint:     "lb1",
				Applications: []string{"app1", "app2"},
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: LoadBalancerRelaysResponse{
				Count:    RelayCounts{},
				From:     now,
				To:       now,
				Endpoint: "internal meter error",
			},
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "LoadBalancer not found returns a not found response",
			meterResponse: LoadBalancerRelaysResponse{
				Count:    RelayCounts{},
				From:     now,
				To:       now,
				Endpoint: "non-existent-load-balancer",
			},
			meterErr:           ErrLoadBalancerNotFound,
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "Bad request returns reqest error response",
			meterErr:           InvalidRequest,
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMeter := fakeRelayMeter{
				loadbalancerRelaysResponse: tc.meterResponse,
				responseErr:                tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/endpoints/lb1?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleLoadBalancerRelays(context.Background(), &fakeMeter, logger.New(), "lb1", w, req)

			if !fakeMeter.requestedFrom.Equal(now) {
				t.Fatalf("Expected %v on 'from' parameter, got: %v", now, fakeMeter.requestedFrom)
			}

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			// TODO: Should we return json even if there is a request/internal server error?
			if tc.expectedStatusCode != http.StatusOK {
				return
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: %s, got: %s", "application/json", resp.Header.Get("Content-Type"))
			}

			var r LoadBalancerRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
			}
			if r.Count != tc.meterResponse.Count {
				t.Errorf("Expected Count: %d, got: %d", tc.meterResponse.Count, r.Count)
			}
		})
	}
}

func TestHandleAllLoadBalancersRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      []LoadBalancerRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: []LoadBalancerRelaysResponse{
				{
					Count:        RelayCounts{Success: 5, Failure: 3},
					From:         now,
					To:           now,
					Endpoint:     "lb1",
					Applications: []string{"app1", "app2"},
				},
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: []LoadBalancerRelaysResponse{
				{
					Count:    RelayCounts{},
					From:     now,
					To:       now,
					Endpoint: "internal meter error",
				},
			},
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "LoadBalancer not found returns a not found response",
			meterResponse: []LoadBalancerRelaysResponse{
				{
					Count:    RelayCounts{},
					From:     now,
					To:       now,
					Endpoint: "non-existent-load-balancer",
				},
			},
			meterErr:           ErrLoadBalancerNotFound,
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "Bad request returns reqest error response",
			meterErr:           InvalidRequest,
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMeter := fakeRelayMeter{
				allLoadBalancersResponse: tc.meterResponse,
				responseErr:              tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/endpoints/lb1?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleAllLoadBalancersRelays(context.Background(), &fakeMeter, logger.New(), w, req)

			if !fakeMeter.requestedFrom.Equal(now) {
				t.Fatalf("Expected %v on 'from' parameter, got: %v", now, fakeMeter.requestedFrom)
			}

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			// TODO: Should we return json even if there is a request/internal server error?
			if tc.expectedStatusCode != http.StatusOK {
				return
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: %s, got: %s", "application/json", resp.Header.Get("Content-Type"))
			}

			var r []LoadBalancerRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
			}
			if diff := cmp.Diff(tc.meterResponse, r); diff != "" {
				t.Errorf("unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeRelayMeter struct {
	requestedFrom time.Time
	requestedTo   time.Time
	requestedApp  string

	response                   AppRelaysResponse
	allResponse                []AppRelaysResponse
	loadbalancerRelaysResponse LoadBalancerRelaysResponse
	allLoadBalancersResponse   []LoadBalancerRelaysResponse
	allClassificationsResponse []OriginClassificationsResponse
	responseErr                error
	latencyResponse            AppLatencyResponse
	allLatencyResponse         []AppLatencyResponse
}

func (f *fakeRelayMeter) AppRelays(ctx context.Context, app string, from, to time.Time) (AppRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	f.requestedApp = app

	return f.response, f.responseErr
}

func (f *fakeRelayMeter) AllAppsRelays(ctx context.Context, from, to time.Time) ([]AppRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to

	return f.allResponse, f.responseErr
}

func (f *fakeRelayMeter) UserRelays(ctx context.Context, user string, from, to time.Time) (UserRelaysResponse, error) {
	return UserRelaysResponse{}, nil
}

func (f *fakeRelayMeter) TotalRelays(ctx context.Context, from, to time.Time) (TotalRelaysResponse, error) {
	return TotalRelaysResponse{}, nil
}

func (f *fakeRelayMeter) LoadBalancerRelays(ctx context.Context, endpoint string, from, to time.Time) (LoadBalancerRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.loadbalancerRelaysResponse, f.responseErr
}

func (f *fakeRelayMeter) AllLoadBalancersRelays(ctx context.Context, from, to time.Time) ([]LoadBalancerRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.allLoadBalancersResponse, f.responseErr
}

func (f *fakeRelayMeter) AllRelaysOrigin(ctx context.Context, from, to time.Time) ([]OriginClassificationsResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.allClassificationsResponse, f.responseErr
}

func (f *fakeRelayMeter) RelaysOrigin(ctx context.Context, origin string, from, to time.Time) (OriginClassificationsResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.allClassificationsResponse[0], f.responseErr
}

func (f *fakeRelayMeter) AllAppsLatencies(ctx context.Context) ([]AppLatencyResponse, error) {
	return f.allLatencyResponse, f.responseErr
}

func (f *fakeRelayMeter) AppLatency(ctx context.Context, app string) (AppLatencyResponse, error) {
	f.requestedApp = app
	return f.latencyResponse, f.responseErr
}

func TestTimePeriod(t *testing.T) {
	// Convert to time.RFC3339, i.e. the maximum granularity for our routines, before using the timestamp
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name         string
		from         string
		to           string
		expectedFrom time.Time
		expectedTo   time.Time
		expectedErr  bool
	}{
		{
			name:         "Both parameters specified correctly",
			from:         now.Format(time.RFC3339),
			to:           now.Add(96 * time.Hour).Format(time.RFC3339),
			expectedFrom: now,
			expectedTo:   now.Add(96 * time.Hour),
		},
		{
			name:       "Missing 'from' parameter returns empty",
			to:         now.Format(time.RFC3339),
			expectedTo: now,
		},
		{
			name:         "Missing 'to' parameter returns empty",
			from:         now.Format(time.RFC3339),
			expectedFrom: now,
		},
		{
			name:        "Invalid 'from' parameter returns error",
			from:        "invalid-date",
			expectedErr: true,
		},
		{
			name:        "Invalid 'to' parameter returns error",
			from:        "invalid-date",
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/apps/app1?from=%s&to=%s", url.QueryEscape(tc.from), url.QueryEscape(tc.to))
			req := httptest.NewRequest("GET", url, nil)
			gotFrom, gotTo, err := timePeriod(req)
			if err != nil {
				if !tc.expectedErr {
					t.Fatalf("Unexpected error: %v", err)
				}
				return
			}
			if !tc.expectedFrom.Equal(gotFrom) {
				t.Errorf("Expected: %v, got: %v", tc.expectedFrom, gotFrom)
			}
			if !gotTo.Equal(tc.expectedTo) {
				t.Errorf("Expected: %v, got: %v", tc.expectedTo, gotTo)
			}
		})
	}
}
