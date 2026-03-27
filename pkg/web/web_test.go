package web

import (
	"fmt"
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
		"explore.html",
		"trending.html",
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

func TestFormatRelativeTimeWeeksAndYears(t *testing.T) {
	t.Parallel()

	// For "weeks" range (7d < d < 365d), output is like "Jan 2".
	weeksAgo := time.Now().Add(-14 * 24 * time.Hour).UnixMilli()
	got := formatRelativeTime(weeksAgo)
	expected := time.UnixMilli(weeksAgo).Format("Jan 2")
	if got != expected {
		t.Errorf("formatRelativeTime(2 weeks ago) = %q, want %q", got, expected)
	}

	// For "years" range (d >= 365d), output is like "Jan 2, 2006".
	yearAgo := time.Now().Add(-400 * 24 * time.Hour).UnixMilli()
	got = formatRelativeTime(yearAgo)
	expected = time.UnixMilli(yearAgo).Format("Jan 2, 2006")
	if got != expected {
		t.Errorf("formatRelativeTime(>1 year ago) = %q, want %q", got, expected)
	}
}

func TestShortenHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"long hex", "abcdef1234567890abcdef1234567890", "abcdef12...7890"},
		{"exactly 17 chars", "abcdef12345678901", "abcdef12...8901"},
		{"exactly 16 chars", "abcdef1234567890", "abcdef1234567890"},
		{"short hex", "abcdef", "abcdef"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shortenHex(tt.input)
			if got != tt.expected {
				t.Errorf("shortenHex(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetInitial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ascii name", "Alice", "A"},
		{"lowercase", "bob", "b"},
		{"empty string", "", "?"},
		{"unicode", "\u00e9mile", "\u00e9"},
		{"emoji", "\U0001f600test", "\U0001f600"},
		{"single char", "X", "X"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getInitial(tt.input)
			if got != tt.expected {
				t.Errorf("getInitial(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRenderContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantSubstr string
		notSubstr  string
	}{
		{
			name:       "plain text",
			input:      "Hello world",
			wantSubstr: "Hello world",
		},
		{
			name:       "hashtag linkified",
			input:      "Check out #xleaks!",
			wantSubstr: `<a href="/search?q=%23xleaks"`,
		},
		{
			name:       "hashtag text preserved",
			input:      "Check out #xleaks!",
			wantSubstr: `#xleaks</a>`,
		},
		{
			name:       "multiple hashtags",
			input:      "#hello and #world",
			wantSubstr: `#hello</a>`,
		},
		{
			name:       "HTML is escaped",
			input:      `<script>alert("xss")</script>`,
			wantSubstr: "&lt;script&gt;",
			notSubstr:  "<script>",
		},
		{
			name:       "HTML in hashtag context",
			input:      `<b>#safe</b>`,
			wantSubstr: "&lt;b&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(renderContent(tt.input))
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("renderContent(%q) = %q, want to contain %q", tt.input, got, tt.wantSubstr)
			}
			if tt.notSubstr != "" && strings.Contains(got, tt.notSubstr) {
				t.Errorf("renderContent(%q) = %q, should NOT contain %q", tt.input, got, tt.notSubstr)
			}
		})
	}
}

func TestMustDecodeHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		wantHex string
	}{
		{"valid hex", "deadbeef", "deadbeef"},
		{"empty string", "", ""},
		{"invalid hex returns nil", "zzzz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mustDecodeHex(tt.input)
			gotHex := ""
			if got != nil {
				gotHex = fmt.Sprintf("%x", got)
			}
			if gotHex != tt.wantHex {
				t.Errorf("mustDecodeHex(%q) = %q, want %q", tt.input, gotHex, tt.wantHex)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"bytes", 512, "512 B"},
		{"kilobytes", 2048, "2.0 KB"},
		{"megabytes", 5 * 1024 * 1024, "5.0 MB"},
		{"gigabytes", 3 * 1024 * 1024 * 1024, "3.0 GB"},
		{"zero", 0, "0 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatBytes(tt.input)
			if got != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"seconds", 45, "45s"},
		{"minutes", 300, "5m"},
		{"hours", 3600, "1h"},
		{"hours and minutes", 3660, "1h 1m"},
		{"days", 86400, "1d"},
		{"days and hours", 90000, "1d 1h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatDuration(tt.input)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildWordSlots(t *testing.T) {
	t.Parallel()

	words := []string{"apple", "banana", "cherry", "date", "elder"}
	positions := []int{1, 3}

	slots, posStr := buildWordSlots(words, positions)

	if len(slots) != 5 {
		t.Fatalf("expected 5 slots, got %d", len(slots))
	}

	// Position 0 should have a word.
	if slots[0].Blank || slots[0].Word != "apple" {
		t.Errorf("slot 0: want Word='apple', Blank=false; got Word=%q, Blank=%v", slots[0].Word, slots[0].Blank)
	}
	// Position 1 should be blank.
	if !slots[1].Blank {
		t.Errorf("slot 1: want Blank=true, got Blank=false")
	}
	// Position 2 should have a word.
	if slots[2].Blank || slots[2].Word != "cherry" {
		t.Errorf("slot 2: want Word='cherry', Blank=false; got Word=%q, Blank=%v", slots[2].Word, slots[2].Blank)
	}
	// Position 3 should be blank.
	if !slots[3].Blank {
		t.Errorf("slot 3: want Blank=true, got Blank=false")
	}

	if posStr != "1,3" {
		t.Errorf("posStr = %q, want '1,3'", posStr)
	}
}

func TestPickRandomPositions(t *testing.T) {
	t.Parallel()

	result := pickRandomPositions(10, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(result))
	}

	// Verify sorted.
	for i := 1; i < len(result); i++ {
		if result[i] <= result[i-1] {
			t.Errorf("positions not sorted: %v", result)
			break
		}
	}

	// Verify all within [0, 10).
	for _, pos := range result {
		if pos < 0 || pos >= 10 {
			t.Errorf("position %d out of range [0, 10)", pos)
		}
	}
}

func TestNormalizedSearchQueries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		fn    func(string) string
		want  string
	}{
		{"post search plain", "test query", normalizedPostSearchQuery, "test query"},
		{"post search hashtag", "#xleaks", normalizedPostSearchQuery, "xleaks"},
		{"post search spaces", "  #hello  ", normalizedPostSearchQuery, "#hello"},
		{"user search plain", "alice", normalizedUserSearchQuery, "alice"},
		{"user search at", "@alice", normalizedUserSearchQuery, "alice"},
		{"user search spaces", "  @bob  ", normalizedUserSearchQuery, "@bob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.fn(tt.input)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, tt.want)
			}
		})
	}
}
