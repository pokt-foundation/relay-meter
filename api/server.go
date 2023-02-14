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

	logger "github.com/sirupsen/logrus"
)

const (
	DATE_LAYOUT    = time.RFC3339
	PARAMETER_FROM = "from"
	PARAMETER_TO   = "to"
)

var (
	// TODO: should we limit the length of application public key or user id in the path regexp?
	appsRelaysPath          = regexp.MustCompile(`^/v[0-1]/relays/apps/([[:alnum:]_]+)$`)
	allAppsRelaysPath       = regexp.MustCompile(`^/v[0-1]/relays/apps`)
	usersRelaysPath         = regexp.MustCompile(`^/v[0-1]/relays/users/([[:alnum:]_]+)$`)
	lbRelaysPath            = regexp.MustCompile(`^/v[0-1]/relays/endpoints/([[:alnum:]_]+)$`)
	allLbsRelaysPath        = regexp.MustCompile(`^/v[0-1]/relays/endpoints`)
	totalRelaysPath         = regexp.MustCompile(`^/v[0-1]/relays`)
	originUsagePath         = regexp.MustCompile(`^/v[0-1]/relays/origin-classification`)
	specificOriginUsagePath = regexp.MustCompile(`^/v[0-1]/relays/origin-classification/([[:alnum:]_].*)`)
	appsLatencyPath         = regexp.MustCompile(`^/v[0-1]/latency/apps/([[:alnum:]_]+)$`)
	allAppsLatencyPath      = regexp.MustCompile(`^/v[0-1]/latency/apps`)
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

func handleAppRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, app string, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AppRelays(ctx, app, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllAppsRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllAppsRelays(ctx, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleUserRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, user string, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.UserRelays(ctx, user, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleLoadBalancerRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, endpoint string, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.LoadBalancerRelays(ctx, endpoint, from, to)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllLoadBalancersRelays(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllLoadBalancersRelays(ctx, from, to)
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

func handleAppLatency(ctx context.Context, meter RelayMeter, l *logger.Logger, app string, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AppLatency(ctx, app)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
}

func handleAllAppsLatency(ctx context.Context, meter RelayMeter, l *logger.Logger, w http.ResponseWriter, req *http.Request) {
	meterEndpoint := func(from, to time.Time) (any, error) {
		return meter.AllAppsLatencies(ctx)
	}
	handleEndpoint(ctx, l, meterEndpoint, w, req)
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
		case meterErr != nil && errors.Is(meterErr, ErrLoadBalancerNotFound):
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
		log := l.WithFields(logger.Fields{"Request": *req})

		if strings.HasPrefix(req.URL.Path, "/v1") && !apiKeys[req.Header.Get("Authorization")] {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte("Unauthorized"))
			if err != nil {
				log.Error("Write in Unauthorized request failed")
			}

			return
		}

		if req.Method != http.MethodGet {
			log.Warn("Incorrect request method, expected: " + http.MethodGet)
			http.Error(w, fmt.Sprintf("Incorrect request method, expected: %s, got: %s", http.MethodPost, req.Method), http.StatusBadRequest)
		}

		if appID := match(appsRelaysPath, req.URL.Path); appID != "" {
			handleAppRelays(ctx, meter, l, appID, w, req)
			return
		}

		if userID := match(usersRelaysPath, req.URL.Path); userID != "" {
			handleUserRelays(ctx, meter, l, userID, w, req)
			return
		}

		if lbID := match(lbRelaysPath, req.URL.Path); lbID != "" {
			handleLoadBalancerRelays(ctx, meter, l, lbID, w, req)
			return
		}

		if appID := match(appsLatencyPath, req.URL.Path); appID != "" {
			handleAppLatency(ctx, meter, l, appID, w, req)
			return
		}

		if allAppsRelaysPath.Match([]byte(req.URL.Path)) {
			handleAllAppsRelays(ctx, meter, l, w, req)
			return
		}

		if allLbsRelaysPath.Match([]byte(req.URL.Path)) {
			handleAllLoadBalancersRelays(ctx, meter, l, w, req)
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
