package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Link mirrors a links row. Content is loaded only when RawTitle is blank,
// because the only thing that reads it is the title fallback (and it can be
// a whole article).
type Link struct {
	ID        string
	URL       string
	Note      *string
	Read      int
	Timestamp int64
	UpdatedAt int64
	Content   *string
	RawTitle  *string
}

const linkColumns = `id, url, note, read, "timestamp", updated_at,
	CASE WHEN btrim(COALESCE(raw_title, '')) = '' THEN content END, raw_title`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.URL, &l.Note, &l.Read, &l.Timestamp, &l.UpdatedAt, &l.Content, &l.RawTitle)
	return l, err
}

// ListLinks returns all of the user's links, most recently updated first.
func (s *Store) ListLinks(ctx context.Context, userID int64) ([]Link, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+linkColumns+` FROM links WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
}

// CreateLink inserts an unread link and its queued content job in one
// transaction, and returns the link and the job id.
func (s *Store) CreateLink(ctx context.Context, userID int64, url, note string) (Link, string, error) {
	nowMs := time.Now().UnixMilli()
	link := Link{ID: newUUID(), URL: url, Note: &note, Timestamp: nowMs, UpdatedAt: nowMs}
	jobID := newUUID()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO links (id, user_id, url, note, read, "timestamp", updated_at)
			VALUES ($1, $2, $3, $4, 0, $5, $5)`, link.ID, userID, url, note, nowMs); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO link_content_jobs (id, link_id, status, created_at, updated_at)
			VALUES ($1, $2, 'queued', `+nowUTC+`, `+nowUTC+`)`, jobID, link.ID)
		return err
	})
	if err != nil {
		return Link{}, "", err
	}
	return link, jobID, nil
}

// SetLinkRead sets read to 1 or 0 and bumps updated_at. found is false when
// the link does not exist or belongs to another user.
func (s *Store) SetLinkRead(ctx context.Context, userID int64, id string, read bool) (bool, error) {
	value := 0
	if read {
		value = 1
	}
	tag, err := s.pool.Exec(ctx, `UPDATE links SET read = $3, updated_at = $4 WHERE id = $2 AND user_id = $1`,
		userID, id, value, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// DeleteLink deletes the user's link and its content jobs.
func (s *Store) DeleteLink(ctx context.Context, userID int64, id string) (bool, error) {
	found := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM links WHERE id = $2 AND user_id = $1`, userID, id)
		if err != nil {
			return err
		}
		found = tag.RowsAffected() == 1
		if !found {
			return nil
		}
		_, err = tx.Exec(ctx, `DELETE FROM link_content_jobs WHERE link_id = $1`, id)
		return err
	})
	return found, err
}
