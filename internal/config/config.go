package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Listen           string
	MastodonInstance string
	MeiliURL         string
	MeiliKey         string
	BufferSize       int
}

func Load() *Config {
	cfg := &Config{
		Listen:     getEnv("MINIDON_LISTEN", ":8080"),
		MeiliURL:   getEnv("MINIDON_MEILI_URL", "http://localhost:7700"),
		MeiliKey:   os.Getenv("MINIDON_MEILI_KEY"),
		BufferSize: getEnvInt("MINIDON_BUFFER_SIZE", 500),
	}
	cfg.MastodonInstance = os.Getenv("MINIDON_MASTODON_INSTANCE")
	return cfg
}

func (c *Config) Validate() error {
	if c.MastodonInstance == "" {
		return fmt.Errorf("MINIDON_MASTODON_INSTANCE is required")
	}
	if c.BufferSize <= 0 {
		return fmt.Errorf("MINIDON_BUFFER_SIZE must be positive, got %d", c.BufferSize)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
