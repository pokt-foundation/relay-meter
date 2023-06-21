package api

import (
	"bytes"
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
	"github.com/pokt-foundation/portal-db/v2/types"
	logger "github.com/sirupsen/logrus"
)

func TestGetHttpServer(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	rawInputToSend := []HTTPSourceRelayCountInput{
		{
			PortalAppID: "21",
			Success:     21,
			Error:       7,
		},
	}

	inputToSend, _ := json.Marshal(rawInputToSend)

	testCases := []struct {
		name               string
		url                string
		method             string
		expected           string
		reqInput           []byte
		expectedStatusCode int
		failAuth           bool
	}{
		{
			name:               "Healthcheck",
			url:                "http://relay-meter.pokt.network/",
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "User relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/users/user?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Total relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "All portal apps relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/portal_apps?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Invalid request path returns an error",
			url:                "http://relay-meter.pokt.network/invalid-path",
			method:             http.MethodGet,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Origin usage path is handled correctly",
			url: fmt.Sprint("http://relay-meter.pokt.network/v1/relays/origin-classification",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Upload relay counts is handled correctly",
			url:                "http://relay-meter.pokt.network/v1/relays/counts",
			method:             http.MethodPost,
			reqInput:           inputToSend,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Failed auhtorization",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/portal_apps?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			),
			method:             http.MethodGet,
			failAuth:           true,
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpServer := GetHttpServer(context.Background(), &fakeRelayMeter{}, logger.New(), map[string]bool{"dummy": true})

			req := httptest.NewRequest(tc.method, tc.url, bytes.NewBuffer(tc.reqInput))
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
				if tc.method == http.MethodGet && req.URL.Path != "/" {
					body, _ := io.ReadAll(resp.Body)
					var r PortalAppRelaysResponse
					if err := json.Unmarshal(body, &r); err != nil {
						t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
					}
				}
			}
		})
	}

}

func TestHandlePortalAppRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      PortalAppRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: PortalAppRelaysResponse{
				Count:       RelayCounts{Success: 5, Failure: 3},
				From:        now,
				To:          now,
				PortalAppID: "portal_app_1",
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: PortalAppRelaysResponse{
				Count:       RelayCounts{},
				From:        now,
				To:          now,
				PortalAppID: "internal meter error",
			},
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "PortalApp not found returns a not found response",
			meterResponse: PortalAppRelaysResponse{
				Count:       RelayCounts{},
				From:        now,
				To:          now,
				PortalAppID: "non-existent-load-balancer",
			},
			meterErr:           ErrPortalAppNotFound,
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
				portalAppRelaysResponse: tc.meterResponse,
				responseErr:             tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/portal_apps/portal_app_1?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handlePortalAppRelays(context.Background(), &fakeMeter, logger.New(), "portal_app_1", w, req)

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

			var r PortalAppRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
			}
			if r.Count != tc.meterResponse.Count {
				t.Errorf("Expected Count: %d, got: %d", tc.meterResponse.Count, r.Count)
			}
		})
	}
}

func TestHandleAllPortalAppsRelays(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		meterResponse      []PortalAppRelaysResponse
		meterErr           error
		expectedStatusCode int
	}{
		{
			name: "Correct number of relays is returned",
			meterResponse: []PortalAppRelaysResponse{
				{
					Count:       RelayCounts{Success: 5, Failure: 3},
					From:        now,
					To:          now,
					PortalAppID: "portal_app_1",
				},
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: []PortalAppRelaysResponse{
				{
					Count:       RelayCounts{},
					From:        now,
					To:          now,
					PortalAppID: "internal meter error",
				},
			},
			meterErr:           fmt.Errorf("Internal meter error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "PortalApp not found returns a not found response",
			meterResponse: []PortalAppRelaysResponse{
				{
					Count:       RelayCounts{},
					From:        now,
					To:          now,
					PortalAppID: "non-existent-load-balancer",
				},
			},
			meterErr:           ErrPortalAppNotFound,
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
				allPortalAppsResponse: tc.meterResponse,
				responseErr:           tc.meterErr,
			}

			url := fmt.Sprintf("http://relay-meter.pokt.network/v1/relays/portal_apps/portal_app_1?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleAllPortalAppsRelays(context.Background(), &fakeMeter, logger.New(), w, req)

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

			var r []PortalAppRelaysResponse
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
	requestedFrom      time.Time
	requestedTo        time.Time
	requestedPortalApp types.PortalAppID

	response                   PortalAppRelaysResponse
	allResponse                []PortalAppRelaysResponse
	portalAppRelaysResponse    PortalAppRelaysResponse
	allPortalAppsResponse      []PortalAppRelaysResponse
	allClassificationsResponse []OriginClassificationsResponse
	responseErr                error
	latencyResponse            AppLatencyResponse
	allLatencyResponse         []AppLatencyResponse
}

func (f *fakeRelayMeter) UserRelays(ctx context.Context, user types.UserID, from, to time.Time) (UserRelaysResponse, error) {
	return UserRelaysResponse{}, nil
}

func (f *fakeRelayMeter) TotalRelays(ctx context.Context, from, to time.Time) (TotalRelaysResponse, error) {
	return TotalRelaysResponse{}, nil
}

func (f *fakeRelayMeter) PortalAppRelays(ctx context.Context, portalAppID types.PortalAppID, from, to time.Time) (PortalAppRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.portalAppRelaysResponse, f.responseErr
}

func (f *fakeRelayMeter) AllPortalAppsRelays(ctx context.Context, from, to time.Time) ([]PortalAppRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	return f.allPortalAppsResponse, f.responseErr
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

func (f *fakeRelayMeter) AppLatency(ctx context.Context, portalAppID types.PortalAppID) (AppLatencyResponse, error) {
	f.requestedPortalApp = portalAppID
	return f.latencyResponse, f.responseErr
}

func (f *fakeRelayMeter) WriteHTTPSourceRelayCounts(ctx context.Context, counts []HTTPSourceRelayCount) error {
	return nil
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
