package config

import "flag"

type ServerConfig struct {
	Address string
}

type HandlerConfig struct {
	BaseURL string
}

type Config struct {
	Server  ServerConfig
	Handler HandlerConfig
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Server.Address, "a", "localhost:8080", "address of the HTTP server")
	flag.StringVar(&cfg.Handler.BaseURL, "b", "http://localhost:8080", "base address of the resulting shortened URL")
	flag.Parse()

	return cfg
}
