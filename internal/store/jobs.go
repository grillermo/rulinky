package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// JobStatus returns a content job's status and error message.
func (s *Store) JobStatus(ctx context.Context, id string) (status, errorMessage string, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT status, COALESCE(error_message, '') FROM link_content_jobs WHERE id = $1`, id).
		Scan(&status, &errorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return status, errorMessage, true, nil
}

// Job is a claimed content job. URL is "" when its link no longer exists.
type Job struct {
	ID     string
	LinkID string
	URL    string
}

// ClaimNextJob marks the oldest queued job as running and returns it.
func (s *Store) ClaimNextJob(ctx context.Context) (Job, bool, error) {
	var j Job
	err := s.pool.QueryRow(ctx, `
		WITH next AS (
		  SELECT id FROM link_content_jobs WHERE status = 'queued'
		  ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE link_content_jobs j
		SET status = 'running', error_message = NULL, updated_at = `+nowUTC+`
		FROM next WHERE j.id = next.id
		RETURNING j.id, j.link_id, COALESCE((SELECT url FROM links WHERE links.id = j.link_id), '')`).
		Scan(&j.ID, &j.LinkID, &j.URL)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// FinishJob stores the fetched title and content on the link (without
// touching its updated_at, so it keeps its place in the list) and marks the
// job finished.
func (s *Store) FinishJob(ctx context.Context, jobID, linkID, title, content string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE links SET content = $2, raw_title = $3 WHERE id = $1`, linkID, content, title); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE link_content_jobs SET status = 'finished', updated_at = `+nowUTC+` WHERE id = $1`, jobID)
		return err
	})
}

func (s *Store) FailJob(ctx context.Context, jobID, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE link_content_jobs SET status = 'failed', error_message = $2, updated_at = `+nowUTC+`
		WHERE id = $1`, jobID, message)
	return err
}

// RequeueRunningJobs puts back jobs left running by a process that stopped
// mid-job. Call it once at startup, before the worker claims anything.
func (s *Store) RequeueRunningJobs(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE link_content_jobs SET status = 'queued', updated_at = `+nowUTC+` WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
