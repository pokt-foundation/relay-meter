package cmd

import (
	"github.com/pokt-foundation/relay-meter/db"
	"github.com/pokt-foundation/utils-go/environment"
)

const (
	POSTGRES_USER        = "POSTGRES_USER"
	POSTGRES_PASSWORD    = "POSTGRES_PASSWORD"
	POSTGRES_HOST        = "POSTGRES_HOST"
	POSTGRES_DB          = "POSTGRES_DB"
	POSTGRES_USE_PRIVATE = "POSTGRES_USE_PRIVATE"
	ENABLE_WRITING       = "ENABLE_WRITING"

	TrueStringChar  = "y"
	FalseStringChar = "n"
)

func GatherPostgresOptions() db.PostgresOptions {
	usePrivate := environment.GetString(POSTGRES_USE_PRIVATE, FalseStringChar)
	enableWriting := environment.GetString(ENABLE_WRITING, FalseStringChar)

	return db.PostgresOptions{
		User:          environment.MustGetString(POSTGRES_USER),
		Password:      environment.MustGetString(POSTGRES_PASSWORD),
		Host:          environment.MustGetString(POSTGRES_HOST),
		DB:            environment.MustGetString(POSTGRES_DB),
		UsePrivate:    usePrivate == TrueStringChar,
		EnableWriting: enableWriting == TrueStringChar,
	}
}
