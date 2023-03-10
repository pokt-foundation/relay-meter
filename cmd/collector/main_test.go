package main

import (
	"reflect"
	"testing"

	"github.com/pokt-foundation/relay-meter/cmd"
)

const expectedInfluxVars = 7
const expectedPostgresVars = 4

// Note: This test is intended to be a health test.
func TestGatherOptions(t *testing.T) {
	influxOptions := cmd.GatherInfluxOptions()
	reflectValues := reflect.ValueOf(influxOptions)

	if reflectValues.NumField() != expectedInfluxVars {
		t.Errorf("Failed to gather influxDB vars: Expected %d env vars but got %d", reflectValues.NumField(), expectedInfluxVars)
	}

	postgresOptions := cmd.GatherPostgresOptions()
	reflectValues = reflect.ValueOf(postgresOptions)

	if reflectValues.NumField() != expectedPostgresVars {
		t.Errorf("Failed to gather postgres vars: Expected %d env vars but got %d", reflectValues.NumField(), expectedPostgresVars)
	}
}
