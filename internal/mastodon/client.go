package mastodon

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	mstdn "github.com/mattn/go-mastodon"

	"github.com/amorken/minidon/internal/model"
)

type Client interface {
	Connect(ctx context.Context) error
	Statuses() <-chan *model.Status
	Connected() bool
	Close() error
}

type Config struct {
	Server       string
	ClientID     string
	ClientSecret string
	AccessToken  string
}

func New(cfg Config) (Client, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("mastodon: server is required")
	}
	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("mastodon: access token is required")
	}

	mc := mstdn.NewClient(&mstdn.Config{
		Server:       cfg.Server,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		AccessToken:  cfg.AccessToken,
	})

	return &mastodonClient{
		cfg:    cfg,
		client: mc,
		ws:     mc.NewWSClient(),
		out:    make(chan *model.Status, 256),
		done:   make(chan struct{}),
	}, nil
}

type mastodonClient struct {
	cfg       Config
	client    *mstdn.Client
	ws        *mstdn.WSClient
	out       chan *model.Status
	done      chan struct{}
	closeOnce sync.Once
	isConnected atomic.Bool
}

func (m *mastodonClient) Connected() bool {
	return m.isConnected.Load()
}

func (m *mastodonClient) Connect(ctx context.Context) error {
	go m.stream(ctx)
	return nil
}

func (m *mastodonClient) Statuses() <-chan *model.Status {
	return m.out
}

func (m *mastodonClient) Close() error {
	close(m.done)
	return nil
}

func (m *mastodonClient) closeOut() {
	m.closeOnce.Do(func() { close(m.out) })
}

func (m *mastodonClient) stream(ctx context.Context) {
	backoff := 1 * time.Second

	for {
		select {
		case <-m.done:
			m.closeOut()
			return
		case <-ctx.Done():
			m.closeOut()
			return
		default:
		}

		events, err := m.ws.StreamingWSPublic(ctx, false)
		if err != nil {
			m.isConnected.Store(false)
			slog.Error("mastodon stream connect error", "err", err, "backoff", backoff)
			m.sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = 1 * time.Second
		m.isConnected.Store(true)
		slog.Info("mastodon stream connected", "server", m.cfg.Server)

		if !m.drain(ctx, events) {
			m.isConnected.Store(false)
			m.closeOut()
			return
		}

		m.isConnected.Store(false)
		slog.Warn("mastodon stream disconnected, reconnecting")
	}
}

func (m *mastodonClient) drain(ctx context.Context, events chan mstdn.Event) bool {
	for {
		select {
		case <-m.done:
			return false
		case <-ctx.Done():
			return false
		case ev, ok := <-events:
			if !ok {
				return true
			}
			if update, ok := ev.(*mstdn.UpdateEvent); ok {
				select {
				case m.out <- convertStatus(update.Status):
				default:
					slog.Warn("mastodon output channel full, dropping status", "id", update.Status.ID)
				}
			}
		}
	}
}

func (m *mastodonClient) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	case <-m.done:
	}
}

func nextBackoff(d time.Duration) time.Duration {
	jitter := time.Duration(rand.Int64N(int64(d) / 2))
	d = d + d/2 + jitter
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func convertStatus(s *mstdn.Status) *model.Status {
	if s == nil {
		return nil
	}
	return &model.Status{
		ID:               string(s.ID),
		URI:              s.URI,
		URL:              s.URL,
		CreatedAt:        s.CreatedAt,
		Content:          s.Content,
		Language:         s.Language,
		Account:          convertAccount(&s.Account),
		MediaAttachments: convertAttachments(s.MediaAttachments),
		Tags:             convertTags(s.Tags),
		Reblog:           convertStatus(s.Reblog),
	}
}

func convertAccount(a *mstdn.Account) model.Account {
	return model.Account{
		Acct:        a.Acct,
		DisplayName: a.DisplayName,
		Avatar:      a.Avatar,
	}
}

func convertAttachments(atts []mstdn.Attachment) []model.MediaAttachment {
	out := make([]model.MediaAttachment, len(atts))
	for i, a := range atts {
		out[i] = model.MediaAttachment{
			PreviewURL: a.PreviewURL,
			Type:       a.Type,
		}
	}
	return out
}

func convertTags(tags []mstdn.Tag) []model.Tag {
	out := make([]model.Tag, len(tags))
	for i, t := range tags {
		out[i] = model.Tag{
			Name: t.Name,
		}
	}
	return out
}
