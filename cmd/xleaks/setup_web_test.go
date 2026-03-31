package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCloser struct {
	calls int32
	done  chan struct{}
}

func (f *fakeCloser) Close() error {
	if atomic.AddInt32(&f.calls, 1) == 1 {
		close(f.done)
	}
	return nil
}

func TestCloseOnContextCancelClosesResource(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer := &fakeCloser{done: make(chan struct{})}
	closeOnContextCancel(ctx, closer)

	cancel()

	select {
	case <-closer.done:
	case <-time.After(time.Second):
		t.Fatal("expected closer to be called after context cancellation")
	}

	if got := atomic.LoadInt32(&closer.calls); got != 1 {
		t.Fatalf("Close() calls = %d, want %d", got, 1)
	}
}
