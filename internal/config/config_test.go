package config

import (
	"testing"
	"time"
)

func TestParse_Priority(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		environment map[string]string
		wantAddress string
		wantBaseURL string
		wantStorage string
		wantDSN     string
		wantSecret  string
	}{
		{
			name:        "defaults when nothing is set",
			args:        nil,
			environment: map[string]string{},
			wantAddress: defaultAddress,
			wantBaseURL: defaultBaseURL,
			wantStorage: defaultFileStoragePath,
			wantSecret:  defaultSecretKey,
		},
		{
			name:        "flags win over defaults",
			args:        []string{"-a", "localhost:9090", "-b", "http://localhost:9090", "-f", "/tmp/from-flag.json", "-k", "from-flag"},
			environment: map[string]string{},
			wantAddress: "localhost:9090",
			wantBaseURL: "http://localhost:9090",
			wantStorage: "/tmp/from-flag.json",
			wantSecret:  "from-flag",
		},
		{
			name: "env wins over flags",
			args: []string{"-a", "localhost:9090", "-b", "http://localhost:9090", "-f", "/tmp/from-flag.json", "-k", "from-flag"},
			environment: map[string]string{
				"SERVER_ADDRESS":    "localhost:7070",
				"BASE_URL":          "http://localhost:7070",
				"FILE_STORAGE_PATH": "/tmp/from-env.json",
				"SECRET_KEY":        "from-env",
			},
			wantAddress: "localhost:7070",
			wantBaseURL: "http://localhost:7070",
			wantStorage: "/tmp/from-env.json",
			wantSecret:  "from-env",
		},
		{
			name: "env wins over defaults when no flags given",
			args: nil,
			environment: map[string]string{
				"SERVER_ADDRESS":    "localhost:7070",
				"BASE_URL":          "http://localhost:7070",
				"FILE_STORAGE_PATH": "/tmp/from-env.json",
				"SECRET_KEY":        "from-env",
			},
			wantAddress: "localhost:7070",
			wantBaseURL: "http://localhost:7070",
			wantStorage: "/tmp/from-env.json",
			wantSecret:  "from-env",
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
			wantSecret:  defaultSecretKey,
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
			if cfg.Database.DSN != tt.wantDSN {
				t.Errorf("Database.DSN = %q, want %q", cfg.Database.DSN, tt.wantDSN)
			}
			if cfg.Auth.SecretKey != tt.wantSecret {
				t.Errorf("Auth.SecretKey = %q, want %q", cfg.Auth.SecretKey, tt.wantSecret)
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
	if cfg.Database.DSN != defaultDatabaseDSN {
		t.Errorf("Database.DSN = %q, want %q", cfg.Database.DSN, defaultDatabaseDSN)
	}
	if cfg.Auth.SecretKey != defaultSecretKey {
		t.Errorf("Auth.SecretKey = %q, want %q", cfg.Auth.SecretKey, defaultSecretKey)
	}
}

func TestParse_DeleterPriority(t *testing.T) {
	flagArgs := []string{
		"-deleter-workers", "8",
		"-deleter-max-writers", "9",
		"-deleter-task-buffer", "2",
		"-deleter-batch-size", "50",
		"-deleter-flush-interval", "2s",
		"-deleter-flush-timeout", "3s",
	}
	envVars := map[string]string{
		"DELETER_WORKERS":        "16",
		"DELETER_MAX_WRITERS":    "17",
		"DELETER_TASK_BUFFER":    "4",
		"DELETER_BATCH_SIZE":     "200",
		"DELETER_FLUSH_INTERVAL": "30s",
		"DELETER_FLUSH_TIMEOUT":  "15s",
	}

	defaults := DeleterConfig{
		Workers:       defaultDeleterWorkers,
		MaxWriters:    defaultDeleterMaxWriters,
		TaskBuffer:    defaultDeleterTaskBuffer,
		BatchSize:     defaultDeleterBatchSize,
		FlushInterval: defaultDeleterFlushInterval,
		FlushTimeout:  defaultDeleterFlushTimeout,
	}

	tests := []struct {
		name        string
		args        []string
		environment map[string]string
		want        DeleterConfig
	}{
		{
			name:        "defaults when nothing is set",
			environment: map[string]string{},
			want:        defaults,
		},
		{
			name:        "flags win over defaults",
			args:        flagArgs,
			environment: map[string]string{},
			want: DeleterConfig{
				Workers:       8,
				MaxWriters:    9,
				TaskBuffer:    2,
				BatchSize:     50,
				FlushInterval: 2 * time.Second,
				FlushTimeout:  3 * time.Second,
			},
		},
		{
			name:        "env wins over flags",
			args:        flagArgs,
			environment: envVars,
			want: DeleterConfig{
				Workers:       16,
				MaxWriters:    17,
				TaskBuffer:    4,
				BatchSize:     200,
				FlushInterval: 30 * time.Second,
				FlushTimeout:  15 * time.Second,
			},
		},
		{
			name:        "env wins over defaults when no flags given",
			environment: envVars,
			want: DeleterConfig{
				Workers:       16,
				MaxWriters:    17,
				TaskBuffer:    4,
				BatchSize:     200,
				FlushInterval: 30 * time.Second,
				FlushTimeout:  15 * time.Second,
			},
		},
		{
			name:        "each parameter resolves independently",
			args:        []string{"-deleter-batch-size", "50"},
			environment: map[string]string{"DELETER_WORKERS": "16"},
			want: DeleterConfig{
				Workers:       16,
				MaxWriters:    defaultDeleterMaxWriters,
				TaskBuffer:    defaultDeleterTaskBuffer,
				BatchSize:     50,
				FlushInterval: defaultDeleterFlushInterval,
				FlushTimeout:  defaultDeleterFlushTimeout,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parse(tt.args, tt.environment)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if cfg.Deleter != tt.want {
				t.Errorf("Deleter = %+v, want %+v", cfg.Deleter, tt.want)
			}
		})
	}
}

func TestParse_RejectsUnusableDeleterValues(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		environment map[string]string
	}{
		{name: "zero workers from a flag", args: []string{"-deleter-workers", "0"}},
		{name: "zero workers from the environment", environment: map[string]string{"DELETER_WORKERS": "0"}},
		{name: "negative workers", args: []string{"-deleter-workers", "-1"}},
		{name: "zero max writers", args: []string{"-deleter-max-writers", "0"}},
		{name: "zero max writers from the environment", environment: map[string]string{"DELETER_MAX_WRITERS": "0"}},
		{name: "negative max writers", args: []string{"-deleter-max-writers", "-1"}},
		{name: "negative task buffer", args: []string{"-deleter-task-buffer", "-1"}},
		{name: "zero batch size", args: []string{"-deleter-batch-size", "0"}},
		{name: "zero batch size from the environment", environment: map[string]string{"DELETER_BATCH_SIZE": "0"}},
		{name: "zero flush interval", args: []string{"-deleter-flush-interval", "0s"}},
		{name: "zero flush interval from the environment", environment: map[string]string{"DELETER_FLUSH_INTERVAL": "0s"}},
		{name: "negative flush timeout", args: []string{"-deleter-flush-timeout", "-1s"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			environment := tt.environment
			if environment == nil {
				environment = map[string]string{}
			}

			if _, err := parse(tt.args, environment); err == nil {
				t.Errorf("parse(%v, %v) returned nil error, want a rejection", tt.args, environment)
			}
		})
	}
}

func TestParse_ZeroIsAValueNotAnAbsentSetting(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		environment map[string]string
	}{
		{
			name:        "from a flag",
			args:        []string{"-deleter-task-buffer", "0"},
			environment: map[string]string{},
		},
		{
			name:        "from the environment",
			environment: map[string]string{"DELETER_TASK_BUFFER": "0"},
		},
		{
			name:        "environment zero wins over a flag",
			args:        []string{"-deleter-task-buffer", "7"},
			environment: map[string]string{"DELETER_TASK_BUFFER": "0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parse(tt.args, tt.environment)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if cfg.Deleter.TaskBuffer != 0 {
				t.Errorf("Deleter.TaskBuffer = %d, want 0", cfg.Deleter.TaskBuffer)
			}
			if cfg.Deleter.Workers != defaultDeleterWorkers {
				t.Errorf("Deleter.Workers = %d, want the default %d", cfg.Deleter.Workers, defaultDeleterWorkers)
			}
		})
	}
}
