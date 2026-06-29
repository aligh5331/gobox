// Package logger provides a structured JSON logger backed by zerolog.
package logger

import (
	"os"

	"github.com/rs/zerolog"
)

// New creates a new zerolog.Logger that writes structured JSON to stdout.
// The level parameter accepts: debug, info, warn, error. Defaults to info.
func New(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(os.Stdout).
		Level(lvl).
		With().
		Timestamp().
		Caller().
		Logger()
}
