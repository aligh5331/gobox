// Package logger provides a zerolog-based structured logger.
package logger

import (
	"os"

	"github.com/rs/zerolog"
)

// New creates a new zerolog.Logger writing structured JSON to stdout at the given level.
// Accepted levels: debug, info, warn, error.
//
// By default, output is JSON lines suitable for production log aggregation.
// Set LOG_FORMAT=pretty to enable human-readable colored output for local development.
func New(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	var output zerolog.Logger
	if os.Getenv("LOG_FORMAT") == "pretty" {
		output = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
			Level(lvl).
			With().
			Timestamp().
			Logger()
	} else {
		output = zerolog.New(os.Stdout).
			Level(lvl).
			With().
			Timestamp().
			Logger()
	}

	return output
}
