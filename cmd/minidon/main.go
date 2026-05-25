package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/amorken/minidon"
	"github.com/amorken/minidon/internal/api"
	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/config"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/model"
)

func main() {
	var cfg config.Config
	ctx := kong.Parse(&cfg,
		kong.Name("minidon"),
		kong.Description("A Mastodon public-timeline streaming web app."),
		kong.UsageOnError(),
	)

	var logWriter io.Writer = os.Stdout
	if ctx.Command() == "cli" {
		logWriter = os.Stderr
	}

	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Set up main application context
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	if ctx.Command() == "cli" {
		if err := cfg.ValidateMastodon(); err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			os.Exit(1)
		}

		mClient, err := mastodon.New(mastodon.Config{
			Server:       cfg.MastodonInstance,
			ClientID:     cfg.MastodonClientID,
			ClientSecret: cfg.MastodonClientSecret,
			AccessToken:  cfg.MastodonAccessToken,
			Stream:       cfg.MastodonStream,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize mastodon client: %v\n", err)
			os.Exit(1)
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := mClient.Connect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to stream: %v\n", err)
			os.Exit(1)
		}
		defer mClient.Close()

		slog.Info("starting stream client CLI mode", "format", cfg.Cli.Format)

		statuses := mClient.Statuses()
		for {
			select {
			case <-ctx.Done():
				slog.Info("stopping stream client")
				return
			case status, ok := <-statuses:
				if !ok {
					slog.Info("stream channel closed")
					return
				}
				if err := printStatus(status, cfg.Cli.Format); err != nil {
					slog.Error("failed to print status", "err", err)
				}
			}
		}
	}

	// Web mode (default)
	if err := cfg.ValidateMastodon(); err != nil {
		slog.Warn("configuration warning", "err", err)
	}

	// 1. Initialize Thread-Safe Bounded Ring Buffer
	buf := buffer.New(cfg.BufferSize)
	slog.Info("initialized in-memory ring buffer", "size", cfg.BufferSize)

	// 2. Initialize Search Backend (MeiliSearch or Noop Index)
	idx := index.NewFromConfig(cfg.DisableSearch, cfg.MeiliURL, cfg.MeiliKey)
	var err error

	// Ensure Settings
	if err := idx.EnsureSettings(appCtx); err != nil {
		slog.Warn("failed to ensure search index settings", "err", err)
	}

	// 3. Initialize Mastodon Client (or Fake Client in Dev mode)
	var mClient mastodon.Client
	if cfg.MastodonInstance != "" && cfg.MastodonAccessToken != "" {
		mClient, err = mastodon.New(mastodon.Config{
			Server:       cfg.MastodonInstance,
			ClientID:     cfg.MastodonClientID,
			ClientSecret: cfg.MastodonClientSecret,
			AccessToken:  cfg.MastodonAccessToken,
			Stream:       cfg.MastodonStream,
		})
		if err != nil {
			slog.Error("failed to create Mastodon client; falling back to fake client", "err", err)
			mClient = mastodon.NewFakeClient()
		}
	} else {
		slog.Warn("MINIDON_MASTODON_INSTANCE or ACCESS_TOKEN not set or invalid. Falling back to FakeClient.")
		fake := mastodon.NewFakeClient()
		mClient = fake

		// Seed fake statuses periodically to facilitate developer manual verification without real tokens
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			id := 0
			for {
				select {
				case <-appCtx.Done():
					return
				case <-ticker.C:
					id++
					fake.Send(&model.Status{
						ID:        fmt.Sprintf("fake-%d", id),
						Content:   fmt.Sprintf("<p>This is a simulated status message #%d for developer preview.</p>", id),
						CreatedAt: time.Now(),
						Account: model.Account{
							Acct:        "developer@localhost",
							DisplayName: "Dev Preview",
							Avatar:      "https://robohash.org/minidon",
						},
						Language: "en",
					})
				}
			}
		}()
	}

	// 4. Initialize Ingest Pipeline
	pipeline := ingest.New(mClient.Statuses(), buf, idx)

	// Start pipeline loop
	go pipeline.Start(appCtx)

	// Start mastodon stream connection
	if err := mClient.Connect(appCtx); err != nil {
		slog.Error("failed to connect mastodon stream", "err", err)
		os.Exit(1)
	}

	// 5. Build router and mount API/static Handlers
	mux := api.NewRouter(minidon.StaticFS, buf, idx, pipeline, mClient)

	srv := &http.Server{
		Addr:         cfg.Listen,
		Handler:      mux,
		BaseContext: func(l net.Listener) context.Context {
			return appCtx
		},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting server", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Wait for termination signal
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down HTTP server", "signal", sig)

	// Force immediate exit if a second interrupt signal is received during shutdown
	go func() {
		sig2 := <-quit
		slog.Warn("forced immediate shutdown requested", "signal", sig2)
		os.Exit(1)
	}()

	// 1. Cancel app context first (propagates to active connection request contexts via BaseContext)
	slog.Info("stopping ingest pipeline and active connections")
	appCancel()

	// 2. Gracefully stop HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}

	// 3. Close Mastodon client
	slog.Info("closing mastodon client")
	if err := mClient.Close(); err != nil {
		slog.Error("client close error", "err", err)
	}

	slog.Info("server stopped")
}

func printStatus(s *model.Status, format string) error {
	if s == nil {
		return nil
	}

	if format == "json" {
		data, err := json.Marshal(s)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println(formatStatusText(s, 0))
	return nil
}

func formatStatusText(s *model.Status, indent int) string {
	if s == nil {
		return ""
	}

	indentStr := strings.Repeat("  ", indent)
	var sb strings.Builder

	if indent == 0 {
		sb.WriteString("------------------------------------------------------------\n")
	}

	sb.WriteString(fmt.Sprintf("%sID:        %s\n", indentStr, s.ID))
	sb.WriteString(fmt.Sprintf("%sTime:      %s\n", indentStr, s.CreatedAt.Local().Format("2006-01-02 15:04:05")))

	acct := s.Account.Acct
	if s.Account.DisplayName != "" {
		sb.WriteString(fmt.Sprintf("%sUser:      %s (%s)\n", indentStr, s.Account.DisplayName, acct))
	} else {
		sb.WriteString(fmt.Sprintf("%sUser:      %s\n", indentStr, acct))
	}

	if s.URL != "" {
		sb.WriteString(fmt.Sprintf("%sURL:       %s\n", indentStr, s.URL))
	}

	if len(s.MediaAttachments) > 0 {
		var atts []string
		for _, att := range s.MediaAttachments {
			atts = append(atts, fmt.Sprintf("%s [%s]", att.PreviewURL, att.Type))
		}
		sb.WriteString(fmt.Sprintf("%sMedia:     %s\n", indentStr, strings.Join(atts, ", ")))
	}

	if len(s.Tags) > 0 {
		var tags []string
		for _, t := range s.Tags {
			tags = append(tags, "#"+t.Name)
		}
		sb.WriteString(fmt.Sprintf("%sTags:      %s\n", indentStr, strings.Join(tags, " ")))
	}

	sb.WriteString(fmt.Sprintf("%sContent:   %s\n", indentStr, cleanHTML(s.Content)))

	if s.Reblog != nil {
		sb.WriteString(fmt.Sprintf("%sBOOSTED STATUS:\n", indentStr))
		sb.WriteString(formatStatusText(s.Reblog, indent+1))
	}

	if indent == 0 {
		sb.WriteString("------------------------------------------------------------")
	}

	return sb.String()
}

func cleanHTML(html string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			sb.WriteRune(r)
		}
	}
	s := sb.String()
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")
	return strings.TrimSpace(s)
}
