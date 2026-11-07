package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"rulinky/internal/store"
)

type Crawler interface {
	Crawl(ctx context.Context, pageURL string) (string, error)
}

type Previewer interface {
	Preview(ctx context.Context, html, pageURL string) (Preview, error)
}

// Worker processes content jobs one at a time, so only one Chrome runs at once.
type Worker struct {
	Store        *store.Store
	Crawler      Crawler
	Previewer    Previewer
	Logger       *slog.Logger
	PollInterval time.Duration
	wake         chan struct{}
}

func NewWorker(st *store.Store, c Crawler, p Previewer, logger *slog.Logger) *Worker {
	return &Worker{Store: st, Crawler: c, Previewer: p, Logger: logger, PollInterval: time.Minute, wake: make(chan struct{}, 1)}
}

// Notify wakes the worker after a job was queued. It never blocks.
func (w *Worker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run requeues jobs left running by a previous process, then processes the
// queue until ctx is cancelled. It waits for Notify or PollInterval between
// passes.
func (w *Worker) Run(ctx context.Context) error {
	n, err := w.Store.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		w.Logger.Info("requeued interrupted content jobs", "count", n)
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for {
			processed, err := w.RunOnce(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				w.Logger.Error("content worker", "err", err)
				break
			}
			if !processed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

// RunOnce claims and processes one job. processed is false when the queue is
// empty. A failing job is recorded on the job row, not returned. A job
// interrupted by shutdown stays "running" and is requeued at the next start.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, ok, err := w.Store.ClaimNextJob(ctx)
	if err != nil || !ok {
		return false, err
	}
	w.Logger.Info("content job start", "job_id", job.ID, "url", job.URL)
	title, contentHTML, err := w.fetch(ctx, job)
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if err != nil {
		w.Logger.Error("content job failed", "job_id", job.ID, "err", err)
		return true, w.Store.FailJob(ctx, job.ID, err.Error())
	}
	if err := w.Store.FinishJob(ctx, job.ID, job.LinkID, title, contentHTML); err != nil {
		w.Logger.Error("content job finish failed", "job_id", job.ID, "err", err)
		if failErr := w.Store.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			return true, errors.Join(err, failErr)
		}
		return true, nil
	}
	w.Logger.Info("content job finished", "job_id", job.ID, "title", title, "content_bytes", len(contentHTML))
	return true, nil
}

func (w *Worker) fetch(ctx context.Context, job store.Job) (title, content string, err error) {
	defer func() {
		if r := recover(); r != nil {
			w.Logger.Error("content job panicked", "job_id", job.ID, "panic", r, "stack", string(debug.Stack()))
			title, content, err = "", "", fmt.Errorf("panic: %v", r)
		}
	}()
	if job.URL == "" {
		return "", "", errors.New("link not found")
	}
	html, err := w.Crawler.Crawl(ctx, job.URL)
	if err != nil {
		return "", "", err
	}
	preview, err := w.Previewer.Preview(ctx, html, job.URL)
	if err != nil {
		return "", "", err
	}
	return preview.Title, preview.Content, nil
}
