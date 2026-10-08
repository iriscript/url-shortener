package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func InitLogger() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Info().Str("logLevel", zerolog.GlobalLevel().String()).Msg("Logger initialized successfully")
}
