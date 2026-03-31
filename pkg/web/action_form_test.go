package web

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

func TestHandleLikeRejectsWhitespaceOnlyTarget(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)
	handler.createReaction = func(context.Context, *identity.KeyPair, []byte) error {
		t.Fatal("createReaction should not be called for a missing target")
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/web/like", strings.NewReader("target=+++"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.handleLike(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); body != "missing target\n" {
		t.Fatalf("body = %q, want %q", body, "missing target\n")
	}
}

func TestHandleRepostRequiresRepostHandler(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/web/repost", strings.NewReader("target="+strings.Repeat("a", 64)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.handleRepost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if body := rr.Body.String(); body != "reposts not configured\n" {
		t.Fatalf("body = %q, want %q", body, "reposts not configured\n")
	}
}

func TestHandleSendDMTrimsRecipient(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)
	recipientKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	called := false
	handler.sendDM = func(_ context.Context, _ *identity.KeyPair, recipientPubkey []byte, content string) error {
		called = true
		if !bytes.Equal(recipientPubkey, recipientKP.PublicKeyBytes()) {
			t.Fatalf("recipientPubkey = %x, want %x", recipientPubkey, recipientKP.PublicKeyBytes())
		}
		if content != "hello" {
			t.Fatalf("content = %q, want %q", content, "hello")
		}
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/web/send-dm", strings.NewReader("recipient=+"+hex.EncodeToString(recipientKP.PublicKeyBytes())+"+&content=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.handleSendDM(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("sendDM should be called for a trimmed valid recipient")
	}
	if body := rr.Body.String(); !strings.Contains(body, "hello") {
		t.Fatalf("body = %q, want rendered message content", body)
	}
}
