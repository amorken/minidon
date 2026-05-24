package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Listen               string
	MastodonInstance     string
	MastodonClientID     string
	MastodonClientSecret string
	MastodonAccessToken  string
	MastodonStreamPath   string
	MastodonStream       string
	MeiliURL             string
	MeiliKey             string
	BufferSize           int
	DisableSearch        bool
}

func Load() *Config {
	cfg := &Config{
		Listen:               getEnv("MINIDON_LISTEN", ":8080"),
		MeiliURL:             getEnv("MINIDON_MEILI_URL", "http://localhost:7700"),
		MeiliKey:             os.Getenv("MINIDON_MEILI_KEY"),
		BufferSize:           getEnvInt("MINIDON_BUFFER_SIZE", 500),
		DisableSearch:        getEnvBool("MINIDON_DISABLE_SEARCH", false),
		MastodonInstance:     os.Getenv("MINIDON_MASTODON_INSTANCE"),
		MastodonClientID:     os.Getenv("MINIDON_MASTODON_CLIENT_ID"),
		MastodonClientSecret: os.Getenv("MINIDON_MASTODON_CLIENT_SECRET"),
		MastodonAccessToken:  os.Getenv("MINIDON_MASTODON_ACCESS_TOKEN"),
		MastodonStreamPath:   getEnv("MINIDON_MASTODON_STREAM_PATH", "api/v1/streaming"),
		MastodonStream:       getEnv("MINIDON_MASTODON_STREAM", "user"),
	}
	return cfg
}

func (c *Config) Validate() error {
	if c.MastodonInstance == "" {
		return fmt.Errorf("MINIDON_MASTODON_INSTANCE is required")
	}
	if c.MastodonAccessToken == "" {
		return fmt.Errorf("MINIDON_MASTODON_ACCESS_TOKEN is required")
	}
	if c.MastodonStream != "user" && c.MastodonStream != "public" {
		return fmt.Errorf("MINIDON_MASTODON_STREAM must be 'user' or 'public', got %q", c.MastodonStream)
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

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
