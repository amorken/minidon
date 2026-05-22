package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	mode := flag.String("mode", "web", "execution mode (web or cli)")
	format := flag.String("format", "json", "output format for cli mode (json or text)")
	flag.Parse()

	if *mode != "web" && *mode != "cli" {
		fmt.Fprintf(os.Stderr, "invalid mode: %s. valid options are 'web' or 'cli'\n", *mode)
		os.Exit(1)
	}

	if *format != "json" && *format != "text" {
		fmt.Fprintf(os.Stderr, "invalid format: %s. valid options are 'json' or 'text'\n", *format)
		os.Exit(1)
	}

	cfg := config.Load()

	var logWriter io.Writer = os.Stdout
	if *mode == "cli" {
		logWriter = os.Stderr
	}

	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if *mode == "cli" {
		if err := cfg.Validate(); err != nil {
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

		slog.Info("starting stream client CLI mode", "format", *format)

		statuses := mClient.Statuses()
		slog.Info("got statuses")
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
				if err := printStatus(status, *format); err != nil {
					slog.Error("failed to print status", "err", err)
				}
			}
		}
	}

	// Web mode (default)
	if err := cfg.Validate(); err != nil {
		slog.Warn("configuration warning", "err", err)
	}

	// 1. Initialize Thread-Safe Bounded Ring Buffer
	buf := buffer.New(cfg.BufferSize)
	slog.Info("initialized in-memory ring buffer", "size", cfg.BufferSize)

	// 2. Initialize Search Backend (MeiliSearch or Noop Index)
	var idx index.Index
	var err error
	if cfg.MeiliURL != "" {
		idx, err = index.NewMeiliIndex(cfg.MeiliURL, cfg.MeiliKey)
		if err != nil {
			slog.Error("failed to connect to MeiliSearch; falling back to Noop search index", "err", err)
			idx = &index.NoopIndex{}
		} else {
			slog.Info("connected to MeiliSearch backend", "url", cfg.MeiliURL)
		}
	} else {
		slog.Info("MeiliSearch not configured; using Noop search index")
		idx = &index.NoopIndex{}
	}

	// 3. Initialize Mastodon Client (or Fake Client in Dev mode)
	var client mastodon.Client
	if cfg.MastodonInstance != "" && cfg.MastodonAccessToken != "" {
		client, err = mastodon.New(mastodon.Config{
			Server:       cfg.MastodonInstance,
			ClientID:     cfg.MastodonClientID,
			ClientSecret: cfg.MastodonClientSecret,
			AccessToken:  cfg.MastodonAccessToken,
			Stream:       cfg.MastodonStream,
		})
		if err != nil {
			slog.Error("failed to create Mastodon client; falling back to fake client", "err", err)
			client = mastodon.NewFakeClient()
		} else {
			slog.Info("created Mastodon client", "instance", cfg.MastodonInstance)
		}
	} else {
		slog.Warn("Mastodon credentials missing; starting with fake/dev client")
		client = mastodon.NewFakeClient()
	}

	// 4. Initialize and Start Ingest Pipeline
	pipeline := ingest.NewPipeline(client, buf, idx)
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	if err := pipeline.Start(runCtx); err != nil {
		slog.Error("failed to start ingest pipeline", "err", err)
		os.Exit(1)
	}
	slog.Info("ingest pipeline started successfully")

	// 5. Build Router & Serve
	mux := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	srv := &http.Server{
		Addr:         cfg.Listen,
		Handler:      mux,
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down", "signal", sig)

	// Stop the ingest pipeline and clean up goroutines
	runCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
		os.Exit(1)
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
