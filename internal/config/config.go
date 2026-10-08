package config

import (
	"errors"
	"flag"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog/log"
)

const (
	defaultAddress         = "localhost:8080"
	defaultBaseURL         = "http://localhost:8080"
	defaultFileStoragePath = "/tmp/short-url-db.json"
)

type ServerConfig struct {
	Address string `env:"SERVER_ADDRESS"`
}

type HandlerConfig struct {
	BaseURL string `env:"BASE_URL"`
}

type StorageConfig struct {
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
}

type Config struct {
	Server  ServerConfig
	Handler HandlerConfig
	Storage StorageConfig
}

func New() *Config {
	cfg, err := parse(os.Args[1:], nil)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("config: failed to parse configuration")
	}

	return cfg
}

func parse(args []string, environment map[string]string) (*Config, error) {
	var cfg Config

	envErr := env.ParseWithOptions(&cfg, env.Options{Environment: environment})

	var flagAddr, flagBaseURL, flagFileStoragePath string

	fs := flag.NewFlagSet("shortener", flag.ContinueOnError)
	fs.StringVar(&flagAddr, "a", defaultAddress, "address of the HTTP server")
	fs.StringVar(&flagBaseURL, "b", defaultBaseURL, "base address of the resulting shortened URL")
	fs.StringVar(&flagFileStoragePath, "f", defaultFileStoragePath, "path to the file where shortened URLs are stored")
	flagErr := fs.Parse(args)

	if cfg.Server.Address == "" {
		cfg.Server.Address = flagAddr
	}
	if cfg.Handler.BaseURL == "" {
		cfg.Handler.BaseURL = flagBaseURL
	}
	if cfg.Storage.FileStoragePath == "" {
		cfg.Storage.FileStoragePath = flagFileStoragePath
	}

	if envErr != nil {
		return &cfg, envErr
	}

	return &cfg, flagErr
}
