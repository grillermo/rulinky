// Package store is Rulinky's Postgres access layer. It reads and writes the
// tables the Rails app created; it never migrates them.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nowUTC is how every `timestamp` column is written: Rails stored them as UTC
// wall-clock time without a time zone.
const nowUTC = `(now() AT TIME ZONE 'UTC')`

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the connection pool, for tests that need to arrange rows directly.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() { s.pool.Close() }
