package config_test

import (
	"testing"

	"github.com/alecthomas/kong"
	"github.com/amorken/minidon/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		args          []string
		wantListen    string
		wantBuf       int
		wantStream    string
		wantDisable   bool
		wantVerbose   bool
		wantErr       bool
	}{
		{
			name:        "Defaults",
			wantListen:  ":8080",
			wantBuf:     500,
			wantStream:  "public",
			wantDisable: false,
			wantVerbose: false,
		},
		{
			name: "EnvOverrides",
			env: map[string]string{
				"MINIDON_LISTEN":          ":9090",
				"MINIDON_BUFFER_SIZE":     "1000",
				"MINIDON_MASTODON_STREAM": "public",
				"MINIDON_DISABLE_SEARCH":  "true",
				"MINIDON_VERBOSE":         "true",
			},
			wantListen:  ":9090",
			wantBuf:     1000,
			wantStream:  "public",
			wantDisable: true,
			wantVerbose: true,
		},
		{
			name: "ArgsOverridesEnv",
			env: map[string]string{
				"MINIDON_LISTEN": ":9090",
			},
			args:        []string{"--listen=:9999"},
			wantListen:  ":9999",
			wantBuf:     500,
			wantStream:  "public",
			wantDisable: false,
			wantVerbose: false,
		},
		{
			name:        "ArgsVerboseShort",
			args:        []string{"-v"},
			wantListen:  ":8080",
			wantBuf:     500,
			wantStream:  "public",
			wantDisable: false,
			wantVerbose: true,
		},
		{
			name:        "ArgsVerboseLong",
			args:        []string{"--verbose"},
			wantListen:  ":8080",
			wantBuf:     500,
			wantStream:  "public",
			wantDisable: false,
			wantVerbose: true,
		},
		{
			name: "PublicLocalStream",
			env: map[string]string{
				"MINIDON_MASTODON_STREAM": "public:local",
			},
			wantListen:  ":8080",
			wantBuf:     500,
			wantStream:  "public:local",
			wantDisable: false,
			wantVerbose: false,
		},
		{
			name: "UserLocalStream",
			env: map[string]string{
				"MINIDON_MASTODON_STREAM": "user:local",
			},
			wantListen:  ":8080",
			wantBuf:     500,
			wantStream:  "user:local",
			wantDisable: false,
			wantVerbose: false,
		},
		{
			name: "InvalidStreamEnum",
			env: map[string]string{
				"MINIDON_MASTODON_STREAM": "invalid_stream",
			},
			wantErr: true,
		},
		{
			name: "InvalidDisableSearchBool",
			env: map[string]string{
				"MINIDON_DISABLE_SEARCH": "invalid_bool",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var cfg config.Config
			p, err := kong.New(&cfg)
			if err != nil {
				t.Fatalf("kong.New: %v", err)
			}
			_, err = p.Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("p.Parse error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if cfg.Listen != tt.wantListen {
				t.Errorf("Listen = %q, want %q", cfg.Listen, tt.wantListen)
			}
			if cfg.BufferSize != tt.wantBuf {
				t.Errorf("BufferSize = %d, want %d", cfg.BufferSize, tt.wantBuf)
			}
			if cfg.MastodonStream != tt.wantStream {
				t.Errorf("MastodonStream = %q, want %q", cfg.MastodonStream, tt.wantStream)
			}
			if cfg.DisableSearch != tt.wantDisable {
				t.Errorf("DisableSearch = %v, want %v", cfg.DisableSearch, tt.wantDisable)
			}
			if cfg.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", cfg.Verbose, tt.wantVerbose)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "Valid",
			cfg: config.Config{
				BufferSize: 500,
			},
			wantErr: false,
		},
		{
			name: "ZeroBufferSize",
			cfg: config.Config{
				BufferSize: 0,
			},
			wantErr: true,
		},
		{
			name: "NegativeBufferSize",
			cfg: config.Config{
				BufferSize: -10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ValidateMastodon(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "Valid",
			cfg: config.Config{
				MastodonInstance:    "mastodon.social",
				MastodonAccessToken: "secret-token",
			},
			wantErr: false,
		},
		{
			name: "MissingInstance",
			cfg: config.Config{
				MastodonAccessToken: "secret-token",
			},
			wantErr: true,
		},
		{
			name: "MissingToken",
			cfg: config.Config{
				MastodonInstance: "mastodon.social",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateMastodon()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMastodon() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
