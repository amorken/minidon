package config_test

import (
	"testing"

	"github.com/amorken/minidon/internal/config"
)

func TestLoad_DisableSearch(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		expected bool
		setEnv   bool
	}{
		{"Empty", "", false, false},
		{"True", "true", true, true},
		{"False", "false", false, true},
		{"One", "1", true, true},
		{"Zero", "0", false, true},
		{"Invalid", "invalid", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("MINIDON_DISABLE_SEARCH", tt.envVal)
			}
			cfg := config.Load()
			if cfg.DisableSearch != tt.expected {
				t.Errorf("expected DisableSearch to be %v, got %v", tt.expected, cfg.DisableSearch)
			}
		})
	}
}
