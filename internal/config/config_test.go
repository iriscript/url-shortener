package config

import "testing"

func TestParse_Priority(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		environment map[string]string
		wantAddress string
		wantBaseURL string
		wantStorage string
	}{
		{
			name:        "defaults when nothing is set",
			args:        nil,
			environment: map[string]string{},
			wantAddress: defaultAddress,
			wantBaseURL: defaultBaseURL,
			wantStorage: defaultFileStoragePath,
		},
		{
			name:        "flags win over defaults",
			args:        []string{"-a", "localhost:9090", "-b", "http://localhost:9090", "-f", "/tmp/from-flag.json"},
			environment: map[string]string{},
			wantAddress: "localhost:9090",
			wantBaseURL: "http://localhost:9090",
			wantStorage: "/tmp/from-flag.json",
		},
		{
			name: "env wins over flags",
			args: []string{"-a", "localhost:9090", "-b", "http://localhost:9090", "-f", "/tmp/from-flag.json"},
			environment: map[string]string{
				"SERVER_ADDRESS":    "localhost:7070",
				"BASE_URL":          "http://localhost:7070",
				"FILE_STORAGE_PATH": "/tmp/from-env.json",
			},
			wantAddress: "localhost:7070",
			wantBaseURL: "http://localhost:7070",
			wantStorage: "/tmp/from-env.json",
		},
		{
			name: "env wins over defaults when no flags given",
			args: nil,
			environment: map[string]string{
				"SERVER_ADDRESS":    "localhost:7070",
				"BASE_URL":          "http://localhost:7070",
				"FILE_STORAGE_PATH": "/tmp/from-env.json",
			},
			wantAddress: "localhost:7070",
			wantBaseURL: "http://localhost:7070",
			wantStorage: "/tmp/from-env.json",
		},
		{
			name: "each parameter resolves independently",
			args: []string{"-f", "/tmp/from-flag.json"},
			environment: map[string]string{
				"SERVER_ADDRESS": "localhost:7070",
			},
			wantAddress: "localhost:7070",
			wantBaseURL: defaultBaseURL,
			wantStorage: "/tmp/from-flag.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parse(tt.args, tt.environment)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if cfg.Server.Address != tt.wantAddress {
				t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, tt.wantAddress)
			}
			if cfg.Handler.BaseURL != tt.wantBaseURL {
				t.Errorf("Handler.BaseURL = %q, want %q", cfg.Handler.BaseURL, tt.wantBaseURL)
			}
			if cfg.Storage.FileStoragePath != tt.wantStorage {
				t.Errorf("Storage.FileStoragePath = %q, want %q", cfg.Storage.FileStoragePath, tt.wantStorage)
			}
		})
	}
}

func TestParse_ReturnsDefaultsAlongsideError(t *testing.T) {
	cfg, err := parse([]string{"-unknown-flag"}, map[string]string{})
	if err == nil {
		t.Fatal("parse returned nil error on an unknown flag, want an error")
	}

	if cfg == nil {
		t.Fatal("parse returned nil config, want a usable one")
	}

	if cfg.Server.Address != defaultAddress {
		t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, defaultAddress)
	}
	if cfg.Storage.FileStoragePath != defaultFileStoragePath {
		t.Errorf("Storage.FileStoragePath = %q, want %q", cfg.Storage.FileStoragePath, defaultFileStoragePath)
	}
}
