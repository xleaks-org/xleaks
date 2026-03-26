package indexer

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSearchPostsReturnsIndexedFields(t *testing.T) {
	t.Parallel()

	idx, err := NewSearchIndex(filepath.Join(t.TempDir(), "search.bleve"))
	if err != nil {
		t.Fatalf("NewSearchIndex() error = %v", err)
	}
	defer idx.Close()

	if err := idx.IndexPost("cid-1", "author-1", "remote whistleblower post", []string{"leak"}, time.Now().UnixMilli()); err != nil {
		t.Fatalf("IndexPost() error = %v", err)
	}

	results, total, err := idx.SearchPosts("whistleblower", 0, 10)
	if err != nil {
		t.Fatalf("SearchPosts() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Author != "author-1" {
		t.Fatalf("expected author field, got %q", results[0].Author)
	}
	if results[0].Content != "remote whistleblower post" {
		t.Fatalf("expected content field, got %q", results[0].Content)
	}
	if results[0].Type != "post" {
		t.Fatalf("expected type=post, got %q", results[0].Type)
	}
}

func TestSearchUsersReturnsIndexedFields(t *testing.T) {
	t.Parallel()

	idx, err := NewSearchIndex(filepath.Join(t.TempDir(), "search.bleve"))
	if err != nil {
		t.Fatalf("NewSearchIndex() error = %v", err)
	}
	defer idx.Close()

	if err := idx.IndexProfile("pubkey-1", "Alice Example", "Investigative reporter"); err != nil {
		t.Fatalf("IndexProfile() error = %v", err)
	}

	results, total, err := idx.SearchUsers("Alice", 0, 10)
	if err != nil {
		t.Fatalf("SearchUsers() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "Alice Example" {
		t.Fatalf("expected display name field, got %q", results[0].Name)
	}
	if results[0].Bio != "Investigative reporter" {
		t.Fatalf("expected bio field, got %q", results[0].Bio)
	}
	if results[0].Type != "profile" {
		t.Fatalf("expected type=profile, got %q", results[0].Type)
	}
}
