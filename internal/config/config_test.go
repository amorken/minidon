package config_test

import (
	"os"
	"testing"

	"github.com/amorken/minidon/internal/config"
)

func TestLoad_DisableSearch(t *testing.T) {
	// Clean up environment after test
	origValue, exists := os.LookupEnv("MINIDON_DISABLE_SEARCH")
	defer func() {
		if exists {
			os.Setenv("MINIDON_DISABLE_SEARCH", origValue)
		} else {
			os.Unsetenv("MINIDON_DISABLE_SEARCH")
		}
	}()

	tests := []struct {
		envVal   string
		expected bool
	}{
		{"", false},
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if tt.envVal == "" {
			os.Unsetenv("MINIDON_DISABLE_SEARCH")
		} else {
			os.Setenv("MINIDON_DISABLE_SEARCH", tt.envVal)
		}

		cfg := config.Load()
		if cfg.DisableSearch != tt.expected {
			t.Errorf("expected DisableSearch to be %v when MINIDON_DISABLE_SEARCH=%q, got %v", tt.expected, tt.envVal, cfg.DisableSearch)
		}
	}
}
