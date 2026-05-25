package config

import (
	"fmt"
)

type WebCmd struct{}

type CliCmd struct {
	Format string `kong:"default='json',enum='json,text',help='Output format for cli mode: json or text.'"`
}

type Config struct {
	DisableSearch        bool   `kong:"name='disable-search',env='MINIDON_DISABLE_SEARCH',help='Disable search functionality.'"`
	Listen               string `kong:"env='MINIDON_LISTEN',default=':8080',help='TCP address to listen on.'"`
	MastodonInstance     string `kong:"env='MINIDON_MASTODON_INSTANCE',help='Mastodon instance hostname.'"`
	MastodonClientID     string `kong:"env='MINIDON_MASTODON_CLIENT_ID',help='Mastodon client ID.'"`
	MastodonClientSecret string `kong:"env='MINIDON_MASTODON_CLIENT_SECRET',help='Mastodon client secret.'"`
	MastodonAccessToken  string `kong:"env='MINIDON_MASTODON_ACCESS_TOKEN',help='Mastodon access token.'"`
	MastodonStreamPath   string `kong:"env='MINIDON_MASTODON_STREAM_PATH',default='api/v1/streaming',help='Mastodon streaming API path.'"`
	MastodonStream       string `kong:"env='MINIDON_MASTODON_STREAM',default='public',enum='user,public',help='Mastodon stream type: user or public.'"`
	MeiliURL             string `kong:"env='MINIDON_MEILI_URL',default='http://localhost:7700',help='MeiliSearch base URL.'"`
	MeiliKey             string `kong:"env='MINIDON_MEILI_KEY',help='MeiliSearch API key.'"`
	BufferSize           int    `kong:"env='MINIDON_BUFFER_SIZE',default='500',help='Number of recent statuses to keep in the ring buffer.'"`
	Verbose              bool   `kong:"short='v',name='verbose',env='MINIDON_VERBOSE',help='Enable verbose logging.'"`

	Web WebCmd `kong:"cmd,default='1',help='Run the web application server (default).'"`
	Cli CliCmd `kong:"cmd,help='Run the streaming timeline client CLI.'"`
}

func (c *Config) Validate() error {
	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer size must be positive, got %d", c.BufferSize)
	}
	return nil
}

func (c *Config) ValidateMastodon() error {
	if c.MastodonInstance == "" {
		return fmt.Errorf("MINIDON_MASTODON_INSTANCE (or --mastodon-instance) is required")
	}
	if c.MastodonAccessToken == "" {
		return fmt.Errorf("MINIDON_MASTODON_ACCESS_TOKEN (or --mastodon-access-token) is required")
	}
	return nil
}
