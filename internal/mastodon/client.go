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
	IsConnected() bool
	Close() error
}

type Config struct {
	Server       string
	ClientID     string
	ClientSecret string
	AccessToken  string
	Stream       string
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
	cfg         Config
	client      *mstdn.Client
	ws          *mstdn.WSClient
	out         chan *model.Status
	done        chan struct{}
	closeOnce   sync.Once
	isConnected atomic.Bool
}

func (m *mastodonClient) IsConnected() bool {
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

		var events chan mstdn.Event
		var err error
		if m.cfg.Stream == "public" {
			events, err = m.ws.StreamingWSPublic(ctx, false)
		} else {
			events, err = m.ws.StreamingWSUser(ctx)
		}
		if err != nil {
			m.isConnected.Store(false)
			slog.Error("mastodon stream connect error", "err", err, "backoff", backoff)
			m.sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = 1 * time.Second
		m.isConnected.Store(true)
		slog.Info("mastodon stream connected", "server", m.cfg.Server, "stream", m.cfg.Stream)

		// Concurrent backfill to fill in timeline gaps
		go m.backfill(ctx)

		if !m.drain(ctx, events) {
			m.isConnected.Store(false)
			m.closeOut()
			return
		}

		m.isConnected.Store(false)
		slog.Warn("mastodon stream disconnected, reconnecting")
	}
}

func (m *mastodonClient) backfill(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-m.done:
		return
	default:
	}

	slog.Info("starting mastodon REST backfill", "server", m.cfg.Server, "stream", m.cfg.Stream)
	var statuses []*mstdn.Status
	var err error
	if m.cfg.Stream == "public" {
		statuses, err = m.client.GetTimelinePublic(ctx, false, nil)
	} else {
		statuses, err = m.client.GetTimelineHome(ctx, nil)
	}

	if err != nil {
		slog.Error("mastodon REST backfill error", "err", err)
		return
	}

	slog.Info("mastodon REST backfill fetched statuses", "count", len(statuses))

	// Send statuses from oldest to newest to preserve chronological ordering
	for i := len(statuses) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case m.out <- convertStatus(statuses[i]):
		default:
			slog.Warn("mastodon output channel full during backfill, dropping status", "id", statuses[i].ID)
		}
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
			switch e := ev.(type) {
			case *mstdn.UpdateEvent:
				select {
				case m.out <- convertStatus(e.Status):
				default:
					slog.Warn("mastodon output channel full, dropping status", "id", e.Status.ID)
				}
			case *mstdn.ErrorEvent:
				slog.Error("mastodon stream error event", "err", e.Err)
			default:
				slog.Info("mastodon unhandled event type", "type", fmt.Sprintf("%T", ev))
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
