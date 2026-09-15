package config

import (
	"flag"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog/log"
)

type ServerConfig struct {
	Address string `env:"SERVER_ADDRESS"`
}

type HandlerConfig struct {
	BaseURL string `env:"BASE_URL"`
}

type Config struct {
	Server  ServerConfig
	Handler HandlerConfig
}

func New() *Config {
	var cfg Config

	err := env.Parse(&cfg)
	if err != nil {
		log.Error().Err(err).Msg("config: failed to parse env")
	}

	var flagAddr, flagBaseURL string
	flag.StringVar(&flagAddr, "a", "localhost:8080", "address of the HTTP server")
	flag.StringVar(&flagBaseURL, "b", "http://localhost:8080", "base address of the resulting shortened URL")
	flag.Parse()

	if cfg.Server.Address == "" {
		cfg.Server.Address = flagAddr
	}
	if cfg.Handler.BaseURL == "" {
		cfg.Handler.BaseURL = flagBaseURL
	}

	return &cfg
}
