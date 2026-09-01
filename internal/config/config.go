package config

import "flag"

type Config struct {
	ServerAddress string
	BaseURL       string
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ServerAddress, "a", "localhost:8080", "address of the HTTP server")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "base address of the resulting shortened URL")
	flag.Parse()

	return cfg
}
