package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog/log"
)

const (
	defaultAddress         = "localhost:8080"
	defaultBaseURL         = "http://localhost:8080"
	defaultFileStoragePath = ""
	defaultDatabaseDSN     = ""
	defaultSecretKey       = "url-shortener-dev-secret"

	defaultDeleterWorkers       = 4
	defaultDeleterMaxWriters    = 64
	defaultDeleterTaskBuffer    = 1
	defaultDeleterBatchSize     = 100
	defaultDeleterFlushInterval = 10 * time.Second
	defaultDeleterFlushTimeout  = 5 * time.Second
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

type DatabaseConfig struct {
	DSN string `env:"DATABASE_DSN"`
}

type AuthConfig struct {
	SecretKey string `env:"SECRET_KEY"`
}

type DeleterConfig struct {
	Workers       int           `env:"DELETER_WORKERS"`
	MaxWriters    int           `env:"DELETER_MAX_WRITERS"`
	TaskBuffer    int           `env:"DELETER_TASK_BUFFER"`
	BatchSize     int           `env:"DELETER_BATCH_SIZE"`
	FlushInterval time.Duration `env:"DELETER_FLUSH_INTERVAL"`
	FlushTimeout  time.Duration `env:"DELETER_FLUSH_TIMEOUT"`
}

type Config struct {
	Server   ServerConfig
	Handler  HandlerConfig
	Storage  StorageConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Deleter  DeleterConfig
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
	cfg := Config{
		Server:   ServerConfig{Address: defaultAddress},
		Handler:  HandlerConfig{BaseURL: defaultBaseURL},
		Storage:  StorageConfig{FileStoragePath: defaultFileStoragePath},
		Database: DatabaseConfig{DSN: defaultDatabaseDSN},
		Auth:     AuthConfig{SecretKey: defaultSecretKey},
		Deleter: DeleterConfig{
			Workers:       defaultDeleterWorkers,
			MaxWriters:    defaultDeleterMaxWriters,
			TaskBuffer:    defaultDeleterTaskBuffer,
			BatchSize:     defaultDeleterBatchSize,
			FlushInterval: defaultDeleterFlushInterval,
			FlushTimeout:  defaultDeleterFlushTimeout,
		},
	}

	var flagAddr, flagBaseURL, flagFileStoragePath, flagDatabaseDSN, flagSecretKey string
	var flagDeleterWorkers, flagDeleterMaxWriters, flagDeleterTaskBuffer, flagDeleterBatchSize int
	var flagDeleterFlushInterval, flagDeleterFlushTimeout time.Duration

	fs := flag.NewFlagSet("shortener", flag.ContinueOnError)
	fs.StringVar(&flagAddr, "a", defaultAddress, "address of the HTTP server")
	fs.StringVar(&flagBaseURL, "b", defaultBaseURL, "base address of the resulting shortened URL")
	fs.StringVar(&flagFileStoragePath, "f", defaultFileStoragePath, "path to the file where shortened URLs are stored")
	fs.StringVar(&flagDatabaseDSN, "d", defaultDatabaseDSN, "PostgreSQL connection string")
	fs.StringVar(&flagSecretKey, "k", defaultSecretKey, "secret key used to sign the auth cookie")
	fs.IntVar(&flagDeleterWorkers, "deleter-workers", defaultDeleterWorkers, "how many goroutines write delete batches at once")
	fs.IntVar(&flagDeleterMaxWriters, "deleter-max-writers", defaultDeleterMaxWriters, "upper bound on goroutines feeding the deleter task channel")
	fs.IntVar(&flagDeleterTaskBuffer, "deleter-task-buffer", defaultDeleterTaskBuffer, "capacity of the deleter task channel")
	fs.IntVar(&flagDeleterBatchSize, "deleter-batch-size", defaultDeleterBatchSize, "how many records a delete batch holds")
	fs.DurationVar(&flagDeleterFlushInterval, "deleter-flush-interval", defaultDeleterFlushInterval, "how often a partial delete batch is flushed")
	fs.DurationVar(&flagDeleterFlushTimeout, "deleter-flush-timeout", defaultDeleterFlushTimeout, "time limit of a single delete batch write")
	flagErr := fs.Parse(args)

	passed := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { passed[f.Name] = true })

	if passed["a"] {
		cfg.Server.Address = flagAddr
	}
	if passed["b"] {
		cfg.Handler.BaseURL = flagBaseURL
	}
	if passed["f"] {
		cfg.Storage.FileStoragePath = flagFileStoragePath
	}
	if passed["d"] {
		cfg.Database.DSN = flagDatabaseDSN
	}
	if passed["k"] {
		cfg.Auth.SecretKey = flagSecretKey
	}
	if passed["deleter-workers"] {
		cfg.Deleter.Workers = flagDeleterWorkers
	}
	if passed["deleter-max-writers"] {
		cfg.Deleter.MaxWriters = flagDeleterMaxWriters
	}
	if passed["deleter-task-buffer"] {
		cfg.Deleter.TaskBuffer = flagDeleterTaskBuffer
	}
	if passed["deleter-batch-size"] {
		cfg.Deleter.BatchSize = flagDeleterBatchSize
	}
	if passed["deleter-flush-interval"] {
		cfg.Deleter.FlushInterval = flagDeleterFlushInterval
	}
	if passed["deleter-flush-timeout"] {
		cfg.Deleter.FlushTimeout = flagDeleterFlushTimeout
	}

	envErr := env.ParseWithOptions(&cfg, env.Options{Environment: environment})

	if envErr != nil {
		return &cfg, envErr
	}
	if flagErr != nil {
		return &cfg, flagErr
	}

	return &cfg, cfg.Deleter.validate()
}

func (c DeleterConfig) validate() error {
	if c.Workers <= 0 {
		return fmt.Errorf("config: deleter workers must be positive, got %d", c.Workers)
	}
	if c.MaxWriters <= 0 {
		return fmt.Errorf("config: deleter max writers must be positive, got %d", c.MaxWriters)
	}
	if c.TaskBuffer < 0 {
		return fmt.Errorf("config: deleter task buffer must not be negative, got %d", c.TaskBuffer)
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("config: deleter batch size must be positive, got %d", c.BatchSize)
	}
	if c.FlushInterval <= 0 {
		return fmt.Errorf("config: deleter flush interval must be positive, got %s", c.FlushInterval)
	}
	if c.FlushTimeout <= 0 {
		return fmt.Errorf("config: deleter flush timeout must be positive, got %s", c.FlushTimeout)
	}

	return nil
}
