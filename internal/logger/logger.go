package logger

import (
	"os"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

// InitLogger initializes the global logger
func InitLogger() {
	Log = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
