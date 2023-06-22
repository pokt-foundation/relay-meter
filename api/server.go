package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	logger "github.com/sirupsen/logrus"
)

const (
	DATE_LAYOUT    = time.RFC3339
	PARAMETER_FROM = "from"
	PARAMETER_TO   = "to"
)

var (
	// TODO: should we limit the length of application public key or user id in the path regexp?
	usersRelaysPath         = regexp.MustCompile(`^/v1/relays/users/([[:alnum:]_]+)$`)
	portalAppRelaysPath     = regexp.MustCompile(`^/v1/relays/portal_apps/([[:alnum:]_]+)$`)
	allPortalAppsRelaysPath = regexp.MustCompile(`^/v1/relays/portal_apps`)
	totalRelaysPath         = regexp.MustCompile(`^/v1/relays`)
	originUsagePath         = regexp.MustCompile(`^/v1/relays/origin-classification`)
	specificOriginUsagePath = regexp.MustCompile(`^/v1/relays/origin-classification/([[:alnum:]_].*)`)
	appsLatencyPath         = regexp.MustCompile(`^/v1/latency/portal_apps/([[:alnum:]_]+)$`)
	allAppsLatencyPath      = regexp.MustCompile(`^/v1/latency/portal_apps`)
	relayCountsPath         = regexp.MustCompile(`^/v1/relays/counts`)
)

// TODO: move these custom error codes to the api package
type ApiError error

var (
	AppNotFound    ApiError = fmt.Errorf("Application not found")
	InvalidRequest ApiError = fmt.Errorf("Invalid request")
)

type ErrorResponse struct {
	Message string
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("Relay Meter is up and running!"))
	if err != nil {
		panic(err)
	}
}

func handleUserRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, userID types.UserID, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.UserRelays(ctx, userID, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handlePortalAppRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, portalAppID types.PortalAppID, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.PortalAppRelays(ctx, portalAppID, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllPortalAppsRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllPortalAppsRelays(ctx, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleTotalRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.TotalRelays(ctx, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleSpecificOriginClassification(ctx context.Context, meter RelayMeter, l *logger.Logger, origin string, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.RelaysOrigin(ctx, origin, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleOriginClassification(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllRelaysOrigin(ctx, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAppLatency(ctx context.Context, meter RelayMeter, l *logger.Logger, portalAppID types.PortalAppID, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AppLatency(ctx, portalAppID)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllAppsLatency(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllAppsLatencies(ctx)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleUploadRelayCounts(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)

	var inCounts []HTTPSourceRelayCountInput
	err := decoder.Decode(&inCounts)
	if err != nil {
		l.WithFields(logger.Fields{"error": err}).Warn("Invalid input")
		http.Error(w, fmt.Sprintf("Invalid input: %v", err), http.StatusBadRequest)
		return
	}

	// just permit to add new counters to today
	now := time.Now()
	var counts []HTTPSourceRelayCount
	for _, incount := range inCounts {
		counts = append(counts, HTTPSourceRelayCount{
			PortalAppID: incount.PortalAppID,
			Day:         now,
			Success:     incount.Success,
			Error:       incount.Error,
		})
	}

	err = meter.WriteHTTPSourceRelayCounts(ctx, counts)
	if err != nil {
		l.WithFields(logger.Fields{"error": err}).Warn("Error on DB")
		http.Error(w, fmt.Sprintf("Error on DB: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "counters added")
}

func handleEndpoint(ctx context.Context, l *logger.Logger, meterEndpoint func(from, to time.Time) (any, error), w http.ResponseWriter, req *http.Request) {
	log := l.WithFields(logger.Fields{"Request": req})
	w.Header().Add("Content-Type", "application/json")

	from, to, err := timePeriod(req)
	if err != nil {
		log.WithFields(logger.Fields{"error": err}).Warn("Invalid timespan")
		http.Error(w, fmt.Sprintf("Invalid timespan: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: separate Internal errors from Request errors using custom errors returned by the meter service
	meterResponse, meterErr := meterEndpoint(from, to)
	if meterErr != nil {
		errLogger := l.WithFields(logger.Fields{"error": meterErr})

		switch {
		case meterErr != nil && errors.Is(meterErr, InvalidRequest):
			errLogger.Warn("Invalid request")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusBadRequest)
		case meterErr != nil && errors.Is(meterErr, AppNotFound):
			errLogger.Warn("Invalid request: application not found")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusBadRequest)
		case meterErr != nil && errors.Is(meterErr, ErrPortalAppNotFound):
			errLogger.Warn("Invalid request: portal app not found")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusNotFound)
		default:
			errLogger.Warn("Internal server error")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	bytes, err := json.Marshal(meterResponse)
	if err != nil {
		log.WithFields(logger.Fields{"error": err}).Warn("Internal error marshalling response")
		http.Error(w, fmt.Sprintf("Internal error marshalling the response %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, string(bytes))
}

func timePeriod(req *http.Request) (time.Time, time.Time, error) {
	parse := func(s string) (time.Time, error) {
		return time.Parse(DATE_LAYOUT, s)
	}

	var (
		from time.Time
		to   time.Time
		err  error
	)

	// TODO: the following two sections should be somehow refactored to avoid code duplication
	fromQueryParameter := req.URL.Query().Get(PARAMETER_FROM)
	if fromQueryParameter != "" {
		from, err = parse(fromQueryParameter)
		if err != nil {
			return from, to, err
		}
	}

	toQueryParameter := req.URL.Query().Get(PARAMETER_TO)
	if toQueryParameter != "" {
		to, err = parse(toQueryParameter)
		if err != nil {
			return from, to, err
		}
	}

	return from, to, nil
}

// TODO: Return 404 on Application not found error
// TODO: Return 304, i.e. Not Modified, if relevant
// TODO: 'Accepts' Header in the request
// serves: /relays/portal_apps
func GetHttpServer(ctx context.Context, meter RelayMeter, l *logger.Logger, apiKeys map[string]bool) func(w http.ResponseWriter, req *http.Request) {
	match := func(r *regexp.Regexp, p string) string {
		matches := r.FindStringSubmatch(p)
		if len(matches) != 2 {
			return ""
		}
		return matches[1]
	}

	return func(w http.ResponseWriter, req *http.Request) {
		log := l.WithFields(logger.Fields{"Request": *req})

		if strings.HasPrefix(req.URL.Path, "/v1") && !apiKeys[req.Header.Get("Authorization")] {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte("Unauthorized"))
			if err != nil {
				log.Error("Write in Unauthorized request failed")
			}

			return
		}

		if req.Method == http.MethodGet {
			if req.URL.Path == "/" {
				healthCheck(w, req)
				return
			}

			if userID := match(usersRelaysPath, req.URL.Path); userID != "" {
				handleUserRelays(ctx, meter, l, types.UserID(userID), w, req)
				return
			}

			if portalAppID := match(portalAppRelaysPath, req.URL.Path); portalAppID != "" {
				handlePortalAppRelays(ctx, meter, l, types.PortalAppID(portalAppID), w, req)
				return
			}

			if portalAppID := match(appsLatencyPath, req.URL.Path); portalAppID != "" {
				handleAppLatency(ctx, meter, l, types.PortalAppID(portalAppID), w, req)
				return
			}

			if allPortalAppsRelaysPath.Match([]byte(req.URL.Path)) {
				handleAllPortalAppsRelays(ctx, meter, l, w, req)
				return
			}

			if origin := match(specificOriginUsagePath, req.URL.Path); origin != "" {
				handleSpecificOriginClassification(ctx, meter, l, origin, w, req)
				return
			}

			if originUsagePath.Match([]byte(req.URL.Path)) {
				handleOriginClassification(ctx, meter, l, w, req)
				return
			}

			if totalRelaysPath.Match([]byte(req.URL.Path)) {
				handleTotalRelays(ctx, meter, l, w, req)
				return
			}

			if allAppsLatencyPath.Match([]byte(req.URL.Path)) {
				handleAllAppsLatency(ctx, meter, l, w, req)
				return
			}
		}

		if req.Method == http.MethodPost {
			if relayCountsPath.Match([]byte(req.URL.Path)) {
				handleUploadRelayCounts(ctx, meter, l, w, req)
				return
			}
		}

		log.Warn("Invalid request endpoint")
		bytes, err := json.Marshal(ErrorResponse{Message: fmt.Sprintf("Invalid request path: %s", req.URL.Path)})
		if err != nil {
			log.WithFields(logger.Fields{"error": err}).Warn("Internal error marshalling response")
			http.Error(w, fmt.Sprintf("Internal error marshalling the response %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, string(bytes))
	}
}
