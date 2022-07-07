package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	logger "github.com/sirupsen/logrus"
)

type RelayMeter interface {
	// AppRelays returns total number of relays for the app over the specified time period
	AppRelays(app string, from, to time.Time) (AppRelaysResponse, error)
	// TODO: relays(user, timePeriod): returns total number of relays for all apps of the user over the specified time period (granularity roughly 1 day as a starting point)
	// TODO: totalrelays(timePeriod)
}

const (
	DATE_LAYOUT    = time.RFC3339
	PARAMETER_FROM = "from"
	PARAMETER_TO   = "to"
)

var (
	// TODO: should we limit the length of application public key or user id in the path regexp?
	appsRelaysPath  = regexp.MustCompile(`^/v0/relays/apps/([[:alnum:]]+)$`)
	usersRelaysPath = regexp.MustCompile(`^/v0/relays/users/([[:alnum:]]+)$`)
	totalRelaysPath = regexp.MustCompile(`^/v0/relays/summary`)
)

// TODO: move these custom error codes to the api package
type ApiError error

var (
	AppNotFound    ApiError = fmt.Errorf("Application not found")
	InvalidRequest ApiError = fmt.Errorf("Invalid request")
)

// TODO: move to the meter package
type AppRelaysResponse struct {
	Count       int64
	From        time.Time
	To          time.Time
	Application string
}

type ErrorResponse struct {
	Message string
}

func handleAppRelays(meter RelayMeter, l *logger.Logger, app string, w http.ResponseWriter, req *http.Request) {
	log := l.WithFields(logger.Fields{"Request": req})
	w.Header().Add("Content-Type", "application/json")

	from, to, err := timePeriod(req)
	if err != nil {
		log.WithFields(logger.Fields{"error": err}).Warn("Invalid timespan")
		http.Error(w, fmt.Sprintf("Invalid timespan: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: separate Internal errors from Request errors using custom errors returned by the meter service
	// Note: This is intentionally placed before request error processing: an internal server error is higher priority for reporting.
	appRelays, meterErr := meter.AppRelays(app, from, to)
	bytes, err := json.Marshal(appRelays)
	if err != nil {
		log.WithFields(logger.Fields{"error": err}).Warn("Internal error marshalling response")
		http.Error(w, fmt.Sprintf("Internal error marshalling the response %v", err), http.StatusInternalServerError)
		return
	}

	var errLogger *logger.Entry
	if meterErr != nil {
		errLogger = log.WithFields(logger.Fields{"error": meterErr})
	}
	switch {
	case meterErr != nil && errors.Is(meterErr, InvalidRequest):
		errLogger.Warn("Invalid request")
		w.WriteHeader(http.StatusBadRequest)
	case meterErr != nil && errors.Is(meterErr, AppNotFound):
		errLogger.Warn("Invalid request: application not found")
		w.WriteHeader(http.StatusBadRequest)
	case meterErr != nil:
		errLogger.Warn("Internal server error")
		w.WriteHeader(http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusOK)
	}
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
func GetHttpServer(meter RelayMeter, l *logger.Logger) func(w http.ResponseWriter, req *http.Request) {
	match := func(r *regexp.Regexp, p string) string {
		matches := r.FindStringSubmatch(p)
		if len(matches) != 2 {
			return ""
		}
		return matches[1]
	}

	return func(w http.ResponseWriter, req *http.Request) {
		log := l.WithFields(logger.Fields{"Request": *req})
		if req.Method != http.MethodGet {
			log.Warn("Incorrect request method, expected: " + http.MethodGet)
			http.Error(w, fmt.Sprintf("Incorrect request method, expected: %s, got: %s", http.MethodPost, req.Method), http.StatusBadRequest)
		}

		/*
			if totalRelaysPath.Match([]byte(req.URL.Path)) {
				return handleTotalRelays()
			}*/
		if appID := match(appsRelaysPath, req.URL.Path); appID != "" {
			handleAppRelays(meter, l, appID, w, req)
			return
		}
		/*
			if userID := match(usersRelaysPath, req.URL.Path); userID != "" {
				return handleUsersRelays()
			}*/

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
