package p2p

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestWriteContentResponseStreamsContent(t *testing.T) {
	t.Parallel()

	const payload = "streamed payload"

	var buf bytes.Buffer
	if err := writeContentResponse(&buf, strings.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("writeContentResponse() error = %v", err)
	}

	got := buf.Bytes()
	if len(got) != 4+len(payload) {
		t.Fatalf("response length = %d, want %d", len(got), 4+len(payload))
	}
	if size := binary.BigEndian.Uint32(got[:4]); size != uint32(len(payload)) {
		t.Fatalf("response size prefix = %d, want %d", size, len(payload))
	}
	if body := string(got[4:]); body != payload {
		t.Fatalf("response body = %q, want %q", body, payload)
	}
}

func TestWriteContentResponseRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	err := writeContentResponse(&bytes.Buffer{}, strings.NewReader("x"), int64(maxContentSize)+1)
	if err == nil {
		t.Fatal("expected oversized content to be rejected")
	}
}
