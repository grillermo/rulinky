package store

import (
	"context"
	"testing"
	"time"
)

func TestClaimFinishJobs(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l1, job1, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	time.Sleep(3 * time.Millisecond)
	_, job2, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")

	got, ok, err := st.ClaimNextJob(ctx)
	if err != nil || !ok || got.ID != job1 || got.LinkID != l1.ID || got.URL != "https://a.com" {
		t.Fatalf("first claim: %+v ok=%v err=%v", got, ok, err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job1); status != "running" {
		t.Fatalf("claimed job status = %q", status)
	}
	second, ok, _ := st.ClaimNextJob(ctx)
	if !ok || second.ID != job2 {
		t.Fatalf("second claim: %+v ok=%v", second, ok)
	}
	if _, ok, _ := st.ClaimNextJob(ctx); ok {
		t.Fatal("claimed a job from an empty queue")
	}

	if err := st.FinishJob(ctx, job1, l1.ID, "Title", "<p>Body</p>"); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job1); status != "finished" {
		t.Fatalf("status = %q", status)
	}
	var title, content string
	var updatedAt int64
	_ = st.Pool().QueryRow(ctx, `SELECT raw_title, content, updated_at FROM links WHERE id = $1`, l1.ID).Scan(&title, &content, &updatedAt)
	if title != "Title" || content != "<p>Body</p>" || updatedAt != l1.UpdatedAt {
		t.Fatalf("link after finish: title=%q content=%q updatedAt=%d (want %d)", title, content, updatedAt, l1.UpdatedAt)
	}
}

func TestFailAndRequeueJobs(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	_, job1, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	_, _, _ = st.ClaimNextJob(ctx)

	if err := st.FailJob(ctx, job1, "boom"); err != nil {
		t.Fatal(err)
	}
	if status, msg, _, _ := st.JobStatus(ctx, job1); status != "failed" || msg != "boom" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}

	_, job2, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")
	_, _, _ = st.ClaimNextJob(ctx) // job2 is now "running", as if the process died mid-job
	n, err := st.RequeueRunningJobs(ctx)
	if err != nil || n != 1 {
		t.Fatalf("requeued %d, err=%v", n, err)
	}
	if status, _, _, _ := st.JobStatus(ctx, job2); status != "queued" {
		t.Fatalf("status = %q", status)
	}
}

func TestClaimJobWhoseLinkIsGone(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	if _, err := st.Pool().Exec(ctx, `INSERT INTO link_content_jobs (id, link_id, status, created_at, updated_at) VALUES ('j1', 'missing', 'queued', now(), now())`); err != nil {
		t.Fatal(err)
	}
	job, ok, err := st.ClaimNextJob(ctx)
	if err != nil || !ok || job.URL != "" {
		t.Fatalf("job=%+v ok=%v err=%v", job, ok, err)
	}
}
