package mastodon

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mstdn "github.com/mattn/go-mastodon"

	"github.com/amorken/minidon/internal/model"
)

type Client interface {
	Connect(ctx context.Context) error
	Events() <-chan *model.Event
	IsConnected() bool
	Close() error
	SetSinceID(id string)
	Server() string
	Stream() string
}

type Config struct {
	Server       string
	ClientID     string
	ClientSecret string
	AccessToken  string
	Stream       string
}

func New(cfg Config) (Client, error) {
	cfg.Server = strings.TrimSpace(cfg.Server)
	cfg.AccessToken = strings.TrimSpace(cfg.AccessToken)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)

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
		out:    make(chan *model.Event, 256),
		done:   make(chan struct{}),
	}, nil
}

type mastodonClient struct {
	cfg         Config
	client      *mstdn.Client
	ws          *mstdn.WSClient
	out         chan *model.Event
	done        chan struct{}
	closeOnce   sync.Once
	isConnected atomic.Bool

	muSince     sync.Mutex
	sinceID     string
}

func (m *mastodonClient) IsConnected() bool {
	return m.isConnected.Load()
}

func (m *mastodonClient) Connect(ctx context.Context) error {
	go m.stream(ctx)
	return nil
}

func (m *mastodonClient) Events() <-chan *model.Event {
	return m.out
}

func (m *mastodonClient) Close() error {
	close(m.done)
	return nil
}

func (m *mastodonClient) closeOut() {
	m.closeOnce.Do(func() { close(m.out) })
}

func (m *mastodonClient) SetSinceID(id string) {
	m.muSince.Lock()
	defer m.muSince.Unlock()
	m.sinceID = id
}

func (m *mastodonClient) Server() string {
	return m.cfg.Server
}

func (m *mastodonClient) Stream() string {
	return m.cfg.Stream
}

func (m *mastodonClient) getSinceID() string {
	m.muSince.Lock()
	defer m.muSince.Unlock()
	return m.sinceID
}

func (m *mastodonClient) updateSinceID(id string) {
	m.muSince.Lock()
	defer m.muSince.Unlock()
	if model.IsNewerID(id, m.sinceID) {
		m.sinceID = id
	}
}

func (m *mastodonClient) getTimelinePage(ctx context.Context, pg *mstdn.Pagination) ([]*mstdn.Status, error) {
	switch m.cfg.Stream {
	case "public":
		return m.client.GetTimelinePublic(ctx, false, pg)
	case "public:local":
		return m.client.GetTimelinePublic(ctx, true, pg)
	default:
		return m.client.GetTimelineHome(ctx, pg)
	}
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

		// Note: The go-mastodon package automatically resolves the instance-supplied
		// streaming_api host for its HTTP/SSE streams (streaming.go), but it does NOT
		// do so for WebSocket streams (streaming_ws.go) which statically use c.client.Config.Server.
		// Since minidon uses the WebSocket client, we fetch the instance metadata and pass the
		// custom streaming API URL directly to a new client config.
		wsClient := m.ws
		instance, err := m.client.GetInstance(ctx)
		if err == nil && instance.URLs != nil {
			if streamURL, ok := instance.URLs["streaming_api"]; ok && streamURL != "" {
				streamingConfig := &mstdn.Config{
					Server:       streamURL,
					ClientID:     m.cfg.ClientID,
					ClientSecret: m.cfg.ClientSecret,
					AccessToken:  m.cfg.AccessToken,
				}
				wsClient = mstdn.NewClient(streamingConfig).NewWSClient()
				slog.Debug("using instance-supplied streaming API path", "url", streamURL)
			}
		} else if err != nil {
			slog.Debug("failed to fetch instance-supplied streaming API path, using default", "err", err)
		}

		var events chan mstdn.Event
		switch m.cfg.Stream {
		case "public":
			events, err = wsClient.StreamingWSPublic(ctx, false)
		case "public:local":
			events, err = wsClient.StreamingWSPublic(ctx, true)
		default:
			events, err = wsClient.StreamingWSUser(ctx)
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

	sinceID := m.getSinceID()
	var allStatuses []*mstdn.Status

	if sinceID == "" {
		slog.Info("starting mastodon REST backfill", "server", m.cfg.Server, "stream", m.cfg.Stream)
		var err error
		allStatuses, err = m.getTimelinePage(ctx, nil)
		if err != nil {
			slog.Error("mastodon REST backfill error", "err", err)
			return
		}
	} else {
		slog.Info("starting mastodon REST backfill catch-up", "server", m.cfg.Server, "stream", m.cfg.Stream, "since_id", sinceID)
		var maxID string
		pageCount := 0
		const maxPages = 50
		for pageCount < maxPages {
			select {
			case <-ctx.Done():
				return
			case <-m.done:
				return
			default:
			}

			pg := &mstdn.Pagination{
				SinceID: mstdn.ID(sinceID),
			}
			if maxID != "" {
				pg.MaxID = mstdn.ID(maxID)
			}

			page, err := m.getTimelinePage(ctx, pg)
			if err != nil {
				slog.Error("mastodon REST backfill error in pagination loop", "err", err)
				break
			}
			if len(page) == 0 {
				break
			}

			allStatuses = append(allStatuses, page...)
			pageCount++

			oldestID := string(page[len(page)-1].ID)
			if oldestID == maxID {
				break
			}
			maxID = oldestID
		}
		slog.Info("mastodon REST backfill catch-up finished", "pages_fetched", pageCount, "total_statuses", len(allStatuses))
	}

	slog.Info("mastodon REST backfill writing statuses to output channel", "count", len(allStatuses))

	// Send statuses from oldest to newest to preserve chronological ordering
	for i := len(allStatuses) - 1; i >= 0; i-- {
		status := convertStatus(allStatuses[i])
		m.updateSinceID(status.ID)
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case m.out <- &model.Event{Type: model.EventTypeStatus, Status: status}:
		default:
			slog.Warn("mastodon output channel full during backfill, dropping status", "id", allStatuses[i].ID)
		}
	}
}

func (m *mastodonClient) drain(ctx context.Context, events chan mstdn.Event) bool {
	needsBackfill := false
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
				slog.Debug("received mastodon update event", "id", e.Status.ID)
				status := convertStatus(e.Status)
				m.updateSinceID(status.ID)
				if needsBackfill {
					slog.Info("reconnect detected, triggering REST backfill")
					go m.backfill(ctx)
					needsBackfill = false
				}
				select {
				case m.out <- &model.Event{Type: model.EventTypeStatus, Status: status}:
				default:
					slog.Warn("mastodon output channel full, dropping status", "id", e.Status.ID)
				}
			case *mstdn.UpdateEditEvent:
				slog.Debug("received mastodon update edit event", "id", e.Status.ID)
				select {
				case m.out <- &model.Event{Type: model.EventTypeStatusEdit, Status: convertStatus(e.Status)}:
				default:
					slog.Warn("mastodon output channel full, dropping status edit", "id", e.Status.ID)
				}
			case *mstdn.DeleteEvent:
				slog.Debug("received mastodon delete event", "id", e.ID)
				select {
				case m.out <- &model.Event{Type: model.EventTypeStatusDelete, StatusID: string(e.ID)}:
				default:
					slog.Warn("mastodon output channel full, dropping status delete", "id", e.ID)
				}
			case *mstdn.ErrorEvent:
				slog.Error("mastodon stream error event", "err", e.Err)
				needsBackfill = true
			default:
				slog.Debug("mastodon unhandled event type", "type", fmt.Sprintf("%T", ev))
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
