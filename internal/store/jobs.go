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
