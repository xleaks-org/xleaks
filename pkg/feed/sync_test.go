package feed

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestSyncPublisherErrorDoesNotLeakPubkey(t *testing.T) {
	t.Parallel()

	pubkey := []byte{0xde, 0xad, 0xbe, 0xef}
	syncer := &Syncer{
		replicator: &Replicator{
			OnFetchContent: func(context.Context, string) ([]byte, error) { return nil, nil },
		},
		OnDiscoverContent: func(context.Context, string) ([]string, error) {
			return nil, errors.New("network unavailable")
		},
	}

	err := syncer.SyncPublisher(context.Background(), pubkey)
	if err == nil {
		t.Fatal("expected error")
	}

	pubkeyHex := hex.EncodeToString(pubkey)
	if strings.Contains(err.Error(), pubkeyHex) {
		t.Fatalf("error leaked raw pubkey: %v", err)
	}
	if got := err.Error(); got != "discover publisher content: network unavailable" {
		t.Fatalf("error = %q, want %q", got, "discover publisher content: network unavailable")
	}
}
