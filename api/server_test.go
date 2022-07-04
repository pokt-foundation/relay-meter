package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"
)

func TestGetHttpServer(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))

	testCases := []struct {
		name               string
		url                string
		expected           string
		expectedStatusCode int
	}{
		{
			name: "App relays path is handled correctly",
			url: fmt.Sprintf("http://relay-meter.pokt.network/v0/relays/apps/app?from=%s&to=%s",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpServer := GetHttpServer(&fakeRelayMeter{}, logger.New())

			req := httptest.NewRequest("GET", tc.url, nil)
			w := httptest.NewRecorder()

			httpServer(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			var r AppRelaysResponse
			if err := json.Unmarshal(body, &r); err != nil {
				t.Fatalf("Unexpected error unmarhsalling the response: %v", err)
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
				Count:       5,
				From:        now,
				To:          now,
				Application: "app",
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Error from the meter returns an internal error response",
			meterResponse: AppRelaysResponse{
				Count:       0,
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
				Count:       0,
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
				Count:       0,
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

			url := fmt.Sprintf("http://relay-meter.pokt.network/v0/relays/apps/app?from=%s&to=%s",
				url.QueryEscape(now.Format(time.RFC3339)),
				url.QueryEscape(now.Format(time.RFC3339)),
			)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handleAppRelays(&fakeMeter, logger.New(), "app", w, req)

			if !fakeMeter.requestedFrom.Equal(now) {
				t.Fatalf("Expected %v on 'from' parameter, got: %v", now, fakeMeter.requestedFrom)
			}

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tc.expectedStatusCode, resp.StatusCode)
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

type fakeRelayMeter struct {
	requestedFrom time.Time
	requestedTo   time.Time
	requestedApp  string

	response    AppRelaysResponse
	responseErr error
}

func (f *fakeRelayMeter) AppRelays(app string, from, to time.Time) (AppRelaysResponse, error) {
	f.requestedFrom = from
	f.requestedTo = to
	f.requestedApp = app

	return f.response, f.responseErr
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
			url := fmt.Sprintf("http://relay-meter.pokt.network/v0/relays/apps/app1?from=%s&to=%s", url.QueryEscape(tc.from), url.QueryEscape(tc.to))
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
