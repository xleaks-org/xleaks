package web

import (
	"fmt"
	"html/template"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var hashtagLinkRe = regexp.MustCompile(`#(\w+)`)

const seedPhraseLength = 24

// formatRelativeTime converts a Unix millisecond timestamp to a human-readable
// relative time string (e.g., "2m", "3h", "5d").
func formatRelativeTime(timestampMs int64) string {
	t := time.UnixMilli(timestampMs)
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	}
}

// formatDuration formats seconds as a human-readable duration string.
func formatDuration(secs float64) string {
	d := time.Duration(secs) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h < 24 {
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := h / 24
	rh := h % 24
	if rh > 0 {
		return fmt.Sprintf("%dd %dh", days, rh)
	}
	return fmt.Sprintf("%dd", days)
}

// getInitial returns the first rune of a string as a string.
func getInitial(name string) string {
	if len(name) == 0 {
		return "?"
	}
	r, _ := utf8.DecodeRuneInString(name)
	return string(r)
}

// shortenHex abbreviates a hex string as "first8...last4".
func shortenHex(hex string) string {
	if len(hex) > 16 {
		return hex[:8] + "..." + hex[len(hex)-4:]
	}
	return hex
}

// pickRandomPositions returns n sorted random positions from [0, total).
func pickRandomPositions(total, n int) []int {
	perm := rand.Perm(total)
	result := perm[:n]
	sort.Ints(result)
	return result
}

// buildWordSlots creates WordSlot entries for seed phrase confirmation,
// blanking out the given positions.
func buildWordSlots(words []string, positions []int) ([]WordSlot, string) {
	blankSet := make(map[int]bool, len(positions))
	for _, p := range positions {
		blankSet[p] = true
	}
	slots := make([]WordSlot, len(words))
	for i, word := range words {
		if blankSet[i] {
			slots[i] = WordSlot{Blank: true}
		} else {
			slots[i] = WordSlot{Word: word}
		}
	}
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = strconv.Itoa(p)
	}
	return slots, strings.Join(parts, ",")
}

// renderContent escapes HTML in post content and converts hashtags into
// clickable links that search for the tag.
func renderContent(content string) template.HTML {
	escaped := template.HTMLEscapeString(content)
	result := hashtagLinkRe.ReplaceAllStringFunc(escaped, func(match string) string {
		tag := match[1:]
		return fmt.Sprintf(`<a href="/search?q=%%23%s" class="text-blue-500 hover:underline">#%s</a>`, tag, tag)
	})
	return template.HTML(result)
}
