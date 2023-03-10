package main

import (
	"reflect"
	"testing"

	"github.com/pokt-foundation/relay-meter/cmd"
)

const expectedServerVars = 10
const expectedPostgresVars = 4

// Note: This test is intended to be a health test.
func TestGatherOptions(t *testing.T) {
	serverOptions := gatherOptions()
	reflectValues := reflect.ValueOf(serverOptions)

	if reflectValues.NumField() != expectedServerVars {
		t.Errorf("Failed to gather server vars: Expected %d env vars but got %d", reflectValues.NumField(), expectedServerVars)
	}

	postgresOptions := cmd.GatherPostgresOptions()
	reflectValues = reflect.ValueOf(postgresOptions)

	if reflectValues.NumField() != expectedPostgresVars {
		t.Errorf("Failed to gather postgres vars: Expected %d env vars but got %d", reflectValues.NumField(), expectedPostgresVars)
	}
}
