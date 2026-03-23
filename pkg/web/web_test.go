package web

import (
	"html/template"
	"strings"
	"testing"
	"time"
)

func TestTemplatesParse(t *testing.T) {
	funcMap := templateFuncMap()

	// Test partials parse
	_, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/feed_items.html")
	if err != nil {
		t.Fatalf("failed to parse feed_items partial: %v", err)
	}

	// Test each page template parses with layout
	pageFiles := []string{
		"home.html",
		"onboarding.html",
		"settings.html",
		"post.html",
		"profile.html",
		"notifications.html",
		"messages.html",
		"search.html",
	}

	for _, pf := range pageFiles {
		t.Run(pf, func(t *testing.T) {
			_, err := template.New("layout.html").Funcs(funcMap).ParseFS(
				templateFS,
				"templates/layout.html",
				"templates/"+pf,
			)
			if err != nil {
				t.Fatalf("failed to parse %s with layout: %v", pf, err)
			}
		})
	}
}

func TestTemplatesRender(t *testing.T) {
	funcMap := templateFuncMap()

	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/home.html",
	)
	if err != nil {
		t.Fatalf("failed to parse home template: %v", err)
	}

	data := map[string]interface{}{
		"Active": "home",
		"Title":  "Home",
		"User": &UserInfo{
			DisplayName: "TestUser",
			Address:     "xleaks1test",
			Pubkey:      "abcdef1234567890abcdef1234567890",
			ShortPubkey: "abcdef12...7890",
		},
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to render home template: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "XLeaks") {
		t.Error("rendered HTML missing XLeaks title")
	}
	if !strings.Contains(html, "TestUser") {
		t.Error("rendered HTML missing user display name")
	}
	if !strings.Contains(html, "Home") {
		t.Error("rendered HTML missing Home header")
	}
}

func TestFeedItemsRender(t *testing.T) {
	funcMap := templateFuncMap()

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/feed_items.html")
	if err != nil {
		t.Fatalf("failed to parse feed_items: %v", err)
	}

	// Test with posts
	data := map[string]interface{}{
		"Posts": []PostView{
			{
				ID:            "abc123",
				AuthorName:    "Alice",
				AuthorInitial: "A",
				ShortPubkey:   "abc1...ef90",
				Content:       "Hello world!",
				RelativeTime:  "2m",
				LikeCount:     5,
				ReplyCount:    1,
				RepostCount:   0,
			},
		},
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "feed_items.html", data); err != nil {
		t.Fatalf("failed to render feed_items: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "Alice") {
		t.Error("rendered HTML missing author name")
	}
	if !strings.Contains(html, "Hello world!") {
		t.Error("rendered HTML missing post content")
	}

	// Test with empty posts
	buf.Reset()
	data["Posts"] = []PostView{}
	if err := tmpl.ExecuteTemplate(&buf, "feed_items.html", data); err != nil {
		t.Fatalf("failed to render empty feed_items: %v", err)
	}

	html = buf.String()
	if !strings.Contains(html, "No posts yet") {
		t.Error("rendered HTML missing empty state message")
	}
}

func TestOnboardingRender(t *testing.T) {
	funcMap := templateFuncMap()

	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/onboarding.html",
	)
	if err != nil {
		t.Fatalf("failed to parse onboarding template: %v", err)
	}

	t.Run("NeedsOnboarding", func(t *testing.T) {
		data := map[string]interface{}{
			"Active":          "",
			"Title":           "Get Started",
			"User":            (*UserInfo)(nil),
			"NeedsOnboarding": true,
		}

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		if !strings.Contains(buf.String(), "Create Identity") {
			t.Error("rendered HTML missing Create Identity button")
		}
	})

	t.Run("Locked", func(t *testing.T) {
		data := map[string]interface{}{
			"Active": "",
			"Title":  "Unlock",
			"User":   (*UserInfo)(nil),
			"Locked": true,
		}

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		if !strings.Contains(buf.String(), "Unlock") {
			t.Error("rendered HTML missing Unlock button")
		}
	})

	t.Run("SeedPhrase", func(t *testing.T) {
		data := map[string]interface{}{
			"Active":     "",
			"Title":      "Save Seed Phrase",
			"User":       (*UserInfo)(nil),
			"SeedPhrase": "word1 word2 word3",
			"SeedWords":  []string{"word1", "word2", "word3"},
		}

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		html := buf.String()
		if !strings.Contains(html, "Save Your Seed Phrase") {
			t.Error("rendered HTML missing seed phrase header")
		}
		if !strings.Contains(html, "word1") {
			t.Error("rendered HTML missing seed words")
		}
	})
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name     string
		ts       int64
		expected string
	}{
		{"just now", now - 10*1000, "now"},
		{"minutes", now - 5*60*1000, "5m"},
		{"hours", now - 3*60*60*1000, "3h"},
		{"days", now - 2*24*60*60*1000, "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRelativeTime(tt.ts)
			if got != tt.expected {
				t.Errorf("formatRelativeTime(%d) = %q, want %q", tt.ts, got, tt.expected)
			}
		})
	}
}
