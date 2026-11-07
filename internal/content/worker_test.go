package content

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"rulinky/internal/store"
)

type fakeCrawler struct {
	html string
	err  error
	urls []string
}

func (f *fakeCrawler) Crawl(_ context.Context, u string) (string, error) {
	f.urls = append(f.urls, u)
	return f.html, f.err
}

type fakePreviewer struct {
	gotHTML string
	preview Preview
	err     error
}

func (f *fakePreviewer) Preview(_ context.Context, html, _ string) (Preview, error) {
	f.gotHTML = html
	return f.preview, f.err
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func setupJob(t *testing.T) (*store.Store, store.Link, string) {
	t.Helper()
	st := store.OpenTest(t)
	ctx := context.Background()
	u, err := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	link, jobID, err := st.CreateLink(ctx, u.ID, "https://a.com", "")
	if err != nil {
		t.Fatal(err)
	}
	return st, link, jobID
}

func TestWorkerRunOnceFinishesJob(t *testing.T) {
	st, link, jobID := setupJob(t)
	crawler := &fakeCrawler{html: "<html>rendered</html>"}
	prev := &fakePreviewer{preview: Preview{Title: "Title", Content: "<p>x</p>"}}
	w := NewWorker(st, crawler, prev, discard)

	processed, err := w.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if len(crawler.urls) != 1 || crawler.urls[0] != link.URL || prev.gotHTML != "<html>rendered</html>" {
		t.Fatalf("crawler urls=%v preview html=%q", crawler.urls, prev.gotHTML)
	}
	if status, _, _, _ := st.JobStatus(context.Background(), jobID); status != "finished" {
		t.Fatalf("status = %q", status)
	}
	if processed, _ := w.RunOnce(context.Background()); processed {
		t.Fatal("processed a job from an empty queue")
	}
}

func TestWorkerRunOnceRecordsFailure(t *testing.T) {
	st, _, jobID := setupJob(t)
	w := NewWorker(st, &fakeCrawler{err: errors.New("boom")}, &fakePreviewer{}, discard)

	if processed, err := w.RunOnce(context.Background()); !processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if status, msg, _, _ := st.JobStatus(context.Background(), jobID); status != "failed" || msg != "boom" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}
}

func TestWorkerRunRequeuesThenProcesses(t *testing.T) {
	st, _, jobID := setupJob(t)
	if _, _, err := st.ClaimNextJob(context.Background()); err != nil { // left "running" by a dead process
		t.Fatal(err)
	}
	w := NewWorker(st, &fakeCrawler{html: "h"}, &fakePreviewer{preview: Preview{Title: "T"}}, discard)
	w.PollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		status, _, _, _ := st.JobStatus(context.Background(), jobID)
		if status == "finished" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never finished, status %q", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}

func TestWorkerNotifyNeverBlocks(t *testing.T) {
	w := NewWorker(nil, nil, nil, discard)
	for range 5 {
		w.Notify()
	}
}

func TestWorkerFailsJobWhoseLinkWasDeleted(t *testing.T) {
	st, link, jobID := setupJob(t)
	ctx := context.Background()
	if _, err := st.Pool().Exec(ctx, `DELETE FROM links WHERE id = $1`, link.ID); err != nil {
		t.Fatal(err)
	}
	crawler := &fakeCrawler{html: "h"}
	w := NewWorker(st, crawler, &fakePreviewer{}, discard)

	if processed, err := w.RunOnce(ctx); !processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if len(crawler.urls) != 0 {
		t.Fatalf("crawled a deleted link: %v", crawler.urls)
	}
	if status, msg, _, _ := st.JobStatus(ctx, jobID); status != "failed" || msg != "link not found" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}
}
