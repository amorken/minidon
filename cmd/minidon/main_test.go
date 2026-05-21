package main

import (
	"strings"
	"testing"
	"time"

	"github.com/amorken/minidon/internal/model"
)

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello World</p>", "Hello World"},
		{"&quot;Hello&quot; &amp; &lt;World&gt;", `"Hello" & <World>`},
		{"   <a href=\"#\">Link</a>   ", "Link"},
		{"&#39;test&#39;", "'test'"},
		{"&apos;test&apos;", "'test'"},
	}

	for _, tt := range tests {
		got := cleanHTML(tt.input)
		if got != tt.expected {
			t.Errorf("cleanHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatStatusText(t *testing.T) {
	status := &model.Status{
		ID:        "11234567890",
		CreatedAt: time.Date(2026, 5, 21, 20, 49, 5, 0, time.UTC),
		Account: model.Account{
			Acct:        "johndoe@mastodon.social",
			DisplayName: "John Doe",
		},
		URL:     "https://mastodon.social/@johndoe/11234567890",
		Content: "<p>Hello world! This is a test status.</p>",
		MediaAttachments: []model.MediaAttachment{
			{PreviewURL: "https://example.com/img.png", Type: "image"},
		},
		Tags: []model.Tag{
			{Name: "test"},
		},
	}

	formatted := formatStatusText(status, 0)

	if !strings.Contains(formatted, "ID:        11234567890") {
		t.Error("expected ID to be formatted correctly")
	}
	if !strings.Contains(formatted, "User:      John Doe (johndoe@mastodon.social)") {
		t.Error("expected User to be formatted correctly")
	}
	if !strings.Contains(formatted, "URL:       https://mastodon.social/@johndoe/11234567890") {
		t.Error("expected URL to be formatted")
	}
	if !strings.Contains(formatted, "Media:     https://example.com/img.png [image]") {
		t.Error("expected Media to be formatted")
	}
	if !strings.Contains(formatted, "Tags:      #test") {
		t.Error("expected Tags to be formatted")
	}
	if !strings.Contains(formatted, "Content:   Hello world! This is a test status.") {
		t.Error("expected Content to be formatted without HTML tags")
	}
}

func TestFormatStatusText_Reblog(t *testing.T) {
	status := &model.Status{
		ID:        "11234567891",
		CreatedAt: time.Date(2026, 5, 21, 20, 50, 0, 0, time.UTC),
		Account: model.Account{
			Acct:        "janesmith@mastodon.social",
			DisplayName: "Jane Smith",
		},
		URL:     "https://mastodon.social/@janesmith/11234567891",
		Content: "",
		Reblog: &model.Status{
			ID:        "11234567890",
			CreatedAt: time.Date(2026, 5, 21, 20, 49, 5, 0, time.UTC),
			Account: model.Account{
				Acct:        "johndoe@mastodon.social",
				DisplayName: "John Doe",
			},
			Content: "<p>Original post</p>",
		},
	}

	formatted := formatStatusText(status, 0)

	if !strings.Contains(formatted, "BOOSTED STATUS:") {
		t.Error("expected BOOSTED STATUS section")
	}
	if !strings.Contains(formatted, "  ID:        11234567890") {
		t.Error("expected indented ID for reblogged status")
	}
	if !strings.Contains(formatted, "  Content:   Original post") {
		t.Error("expected indented content for reblogged status")
	}
}
