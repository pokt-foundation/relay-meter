// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.17.0

package postgresdriver

import (
	"time"
)

type HttpSourceRelayCount struct {
	AppPublicKey string    `json:"appPublicKey"`
	Day          time.Time `json:"day"`
	Success      int64     `json:"success"`
	Error        int64     `json:"error"`
}
