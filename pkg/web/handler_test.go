package web

import "testing"

func TestHandlerCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	sessions := NewSessionManager()
	handler, err := NewHandler(nil, nil, nil, sessions)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	if err := handler.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
