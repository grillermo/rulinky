package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestWorkerFailure(t *testing.T) {
	if workerFailure(nil) != nil {
		t.Error("nil should stay nil")
	}
	if workerFailure(context.Canceled) != nil {
		t.Error("context.Canceled is a clean shutdown, not a failure")
	}
	if workerFailure(errors.Join(errors.New("x"), context.Canceled)) != nil {
		t.Error("wrapped Canceled is a clean shutdown")
	}
	boom := errors.New("boom")
	if err := workerFailure(boom); !errors.Is(err, boom) {
		t.Errorf("real error must be kept, got %v", err)
	}
}

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	s := newHTTPServer("3999", http.NewServeMux())
	if s.Addr != ":3999" {
		t.Errorf("addr = %q", s.Addr)
	}
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 {
		t.Errorf("timeouts must all be set: %+v", s)
	}
}
