package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pokt-foundation/portal-db/v2/types"
	"github.com/pokt-foundation/utils-go/logger"
)

const (
	DATE_LAYOUT    = time.RFC3339
	PARAMETER_FROM = "from"
	PARAMETER_TO   = "to"
	HEALTH_CHECK_PATH string = "/healthz"
)

var (
	// TODO: should we limit the length of application public key or userID in the path regexp?
	appsRelaysPath    = regexp.MustCompile(`^/v1/relays/apps/([[:alnum:]_]+)$`)
	allAppsRelaysPath = regexp.MustCompile(`^/v1/relays/apps`)
	usersRelaysPath   = regexp.MustCompile(`^/v1/relays/users/([[:alnum:]_]+)$`)
	// TODO: should we change the path from endpoints to portal_apps?
	lbRelaysPath            = regexp.MustCompile(`^/v1/relays/endpoints/([[:alnum:]_]+)$`)
	allLbsRelaysPath        = regexp.MustCompile(`^/v1/relays/endpoints`)
	totalRelaysPath         = regexp.MustCompile(`^/v1/relays`)
	originUsagePath         = regexp.MustCompile(`^/v1/relays/origin-classification`)
	specificOriginUsagePath = regexp.MustCompile(`^/v1/relays/origin-classification/([[:alnum:]_].*)`)
	appsLatencyPath         = regexp.MustCompile(`^/v1/latency/apps/([[:alnum:]|_]+)$`)
	allAppsLatencyPath      = regexp.MustCompile(`^/v1/latency/apps`)
	relayCountsPath         = regexp.MustCompile(`^/v1/relays/counts`)

	mutex sync.Mutex
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
	_, err := w.Write([]byte("Relay Meter up and running!"))
	if err != nil {
		panic(err)
	}
}

func handleAppRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, appPubKey types.PortalAppPublicKey, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AppRelays(ctx, appPubKey, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllAppsRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllAppsRelays(ctx, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
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

func handleSpecificOriginClassification(ctx context.Context, meter RelayMeter, l *logger.Logger, origin types.PortalAppOrigin, w http.ResponseWriter, req *http.Request) {
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

func handleAppLatency(ctx context.Context, meter RelayMeter, l *logger.Logger, appPubKey types.PortalAppPublicKey, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AppLatency(ctx, appPubKey)
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
		l.Warn("Invalid input",
			slog.String("error", err.Error()),
		)
		http.Error(w, fmt.Sprintf("Invalid input: %v", err), http.StatusBadRequest)
		return
	}

	// just permit to add new counters to today
	now := time.Now()
	var counts []HTTPSourceRelayCount
	for _, incount := range inCounts {
		counts = append(counts, HTTPSourceRelayCount{
			AppPublicKey: incount.AppPublicKey,
			Day:          now,
			Success:      incount.Success,
			Error:        incount.Error,
		})
	}

	l.Info("apiserver: Received handleUploadRelayCounts request",
		slog.Int("app_counts", len(counts)),
	)

	mutex.Lock() // prevent DB deadlock by blocking the request until the last one finishes
	err = meter.WriteHTTPSourceRelayCounts(ctx, counts)
	mutex.Unlock()

	if err != nil {
		l.Warn("Error on DB",
			slog.String("error", err.Error()),
		)
		http.Error(w, fmt.Sprintf("Error on DB: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "counters added")
}

func handleEndpoint(ctx context.Context, l *logger.Logger, meterEndpoint func(from, to time.Time) (any, error), w http.ResponseWriter, req *http.Request) {
	log := l.With(slog.Group("request", "host", req.Host, "method", req.Method, "url", req.URL))
	w.Header().Add("Content-Type", "application/json")

	from, to, err := timePeriod(req)
	if err != nil {
		log.Warn("Invalid timespan",
			slog.String("error", err.Error()),
		)
		http.Error(w, fmt.Sprintf("Invalid timespan: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: separate Internal errors from Request errors using custom errors returned by the meter service
	meterResponse, meterErr := meterEndpoint(from, to)
	if meterErr != nil {
		errLogger := l.With(slog.String("error", meterErr.Error()))

		switch {
		case meterErr != nil && errors.Is(meterErr, InvalidRequest):
			errLogger.Warn("Invalid request")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusBadRequest)
		case meterErr != nil && errors.Is(meterErr, AppNotFound):
			errLogger.Warn("Invalid request: application not found")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusBadRequest)
		case meterErr != nil && errors.Is(meterErr, ErrPortalAppNotFound):
			errLogger.Warn("Invalid request: load balancer not found")
			http.Error(w, fmt.Sprintf("Bad request: %v", meterErr), http.StatusNotFound)
		default:
			errLogger.Warn("Internal server error")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	bytes, err := json.Marshal(meterResponse)
	if err != nil {
		log.Warn("Internal error marshalling response",
			slog.String("error", err.Error()),
		)
		http.Error(w, fmt.Sprintf("Internal error marshalling the response %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(bytes))
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
// serves: /relays/apps
func GetHttpServer(ctx context.Context, meter RelayMeter, l *logger.Logger, apiKeys map[string]bool) func(w http.ResponseWriter, req *http.Request) {
	match := func(r *regexp.Regexp, p string) string {
		matches := r.FindStringSubmatch(p)
		if len(matches) != 2 {
			return ""
		}
		return matches[1]
	}

	return func(w http.ResponseWriter, req *http.Request) {
		log := l.With(slog.Group("request", "host", req.Host, "method", req.Method, "url", req.URL))

		if strings.HasPrefix(req.URL.Path, "/v1") && !apiKeys[req.Header.Get("Authorization")] {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte("Unauthorized"))
			if err != nil {
				log.Error("Write in Unauthorized request failed")
			}

			return
		}

		if req.Method == http.MethodGet {
			if req.URL.Path == HEALTH_CHECK_PATH {
				healthCheck(w, req)
				return
			}

			if appPubKey := match(appsRelaysPath, req.URL.Path); appPubKey != "" {
				handleAppRelays(ctx, meter, l, types.PortalAppPublicKey(appPubKey), w, req)
				return
			}

			if userID := match(usersRelaysPath, req.URL.Path); userID != "" {
				handleUserRelays(ctx, meter, l, types.UserID(userID), w, req)
				return
			}

			if portalAppID := match(lbRelaysPath, req.URL.Path); portalAppID != "" {
				handlePortalAppRelays(ctx, meter, l, types.PortalAppID(portalAppID), w, req)
				return
			}

			if appPubKey := match(appsLatencyPath, req.URL.Path); appPubKey != "" {
				handleAppLatency(ctx, meter, l, types.PortalAppPublicKey(appPubKey), w, req)
				return
			}

			if allAppsRelaysPath.Match([]byte(req.URL.Path)) {
				handleAllAppsRelays(ctx, meter, l, w, req)
				return
			}

			if allLbsRelaysPath.Match([]byte(req.URL.Path)) {
				handleAllPortalAppsRelays(ctx, meter, l, w, req)
				return
			}

			if origin := match(specificOriginUsagePath, req.URL.Path); origin != "" {
				handleSpecificOriginClassification(ctx, meter, l, types.PortalAppOrigin(origin), w, req)
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
			log.Warn("Internal error marshalling response",
				slog.String("error", err.Error()),
			)
			http.Error(w, fmt.Sprintf("Internal error marshalling the response %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, string(bytes))
	}
}
