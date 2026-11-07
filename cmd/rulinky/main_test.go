package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"rulinky/internal/config"
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

func testOpts(t *testing.T, worker func(context.Context) error) (serveOpts, *atomic.Bool) {
	t.Helper()
	stopped := &atomic.Bool{}
	return serveOpts{
		HTTP:   &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()},
		Listen: func(addr string) (net.Listener, error) { return net.Listen("tcp", addr) },
		Worker: worker,
		Stop:   func() { stopped.Store(true) },
		Grace:  2 * time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, stopped
}

func TestServeCleanShutdown(t *testing.T) {
	o, stopped := testOpts(t, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, o) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("clean shutdown must return nil, got %v", err)
	}
	if !stopped.Load() {
		t.Error("Stop (second-signal force quit) must be called after the first signal")
	}
}

func TestServeWithoutWorker(t *testing.T) {
	o, _ := testOpts(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, o) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServeBindFailureNeverStartsWorker(t *testing.T) {
	var started atomic.Bool
	o, _ := testOpts(t, func(ctx context.Context) error { started.Store(true); return nil })
	o.Listen = func(string) (net.Listener, error) { return nil, errors.New("address already in use") }
	err := serve(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("want bind error, got %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if started.Load() {
		t.Error("worker must not start when the port cannot be bound")
	}
}

func TestServeWorkerFailureShutsServerDown(t *testing.T) {
	boom := errors.New("boom")
	o, stopped := testOpts(t, func(ctx context.Context) error { return boom })
	err := serve(context.Background(), o)
	if !errors.Is(err, boom) {
		t.Fatalf("worker failure must propagate, got %v", err)
	}
	if !stopped.Load() {
		t.Error("Stop must be called")
	}
}

func TestServeWaitsForWorkerToFinish(t *testing.T) {
	var finished atomic.Bool
	o, _ := testOpts(t, func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // e.g. Chrome cleanup
		finished.Store(true)
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, o) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !finished.Load() {
		t.Error("serve returned before the worker finished")
	}
}

func TestServeGivesUpOnStuckWorker(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	o, _ := testOpts(t, func(ctx context.Context) error { <-release; return nil })
	o.Grace = 150 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, o) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Error("a worker that never stops must produce an error")
	}
}

func TestConfigSummaryNeverLeaksSecrets(t *testing.T) {
	for _, dsn := range []string{
		`host='db.local' port='5432' user='u' password='s3cret pw' dbname='rulinky'`,
		`postgres://u:s3cret@db.local:5432/rulinky?sslmode=disable`,
	} {
		out := fmt.Sprint(configSummary(config.Config{DatabaseURL: dsn, Port: "3002", SessionSecret: []byte("topsecret"), SlackOTPWebhookURL: "https://hooks/x"}))
		if !strings.Contains(out, "db.local") || !strings.Contains(out, "rulinky") {
			t.Errorf("summary missing host/name: %s", out)
		}
		for _, bad := range []string{"s3cret", "topsecret", "hooks"} {
			if strings.Contains(out, bad) {
				t.Errorf("summary leaks %q: %s", bad, out)
			}
		}
	}
}
