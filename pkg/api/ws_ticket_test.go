package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWSTicketManagerConsumesTicketsOnce(t *testing.T) {
	t.Parallel()

	m := NewWSTicketManager(time.Minute)
	ticket, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if !m.ValidateAndConsume(ticket) {
		t.Fatal("expected issued ticket to validate")
	}
	if m.ValidateAndConsume(ticket) {
		t.Fatal("expected consumed ticket to be rejected")
	}
}

func TestWSTicketManagerRejectsExpiredTickets(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewWSTicketManager(time.Minute)
	m.now = func() time.Time { return now }

	ticket, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	now = now.Add(2 * time.Minute)
	if m.ValidateAndConsume(ticket) {
		t.Fatal("expected expired ticket to be rejected")
	}
}

func TestWSTicketIssueHandlerDisablesCaching(t *testing.T) {
	t.Parallel()

	m := NewWSTicketManager(time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/api/ws-ticket", nil)
	rr := httptest.NewRecorder()

	m.IssueHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store, max-age=0")
	}

	var payload struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Ticket == "" {
		t.Fatal("expected ticket in response")
	}
}
