package store

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID       int64
	Email    string
	APIToken string
}

// FindOrCreateUserByEmail returns the user with this (already normalized)
// email, creating it if needed, and makes sure it has an API token.
func (s *Store) FindOrCreateUserByEmail(ctx context.Context, email string) (User, error) {
	token, err := newAPIToken()
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (email, api_token, created_at, updated_at)
		VALUES ($1, $2, `+nowUTC+`, `+nowUTC+`)
		ON CONFLICT (email) DO UPDATE SET api_token = COALESCE(users.api_token, EXCLUDED.api_token)
		RETURNING id, email, api_token`, email, token).Scan(&u.ID, &u.Email, &u.APIToken)
	return u, err
}

// SetOTP stores a fresh one-time code that expires in 10 minutes.
func (s *Store) SetOTP(ctx context.Context, userID int64, code string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET otp_code = $2, otp_expires_at = `+nowUTC+` + interval '10 minutes', updated_at = `+nowUTC+`
		WHERE id = $1`, userID, code)
	return err
}

// VerifyOTP checks the code for email. On success it consumes the code, sets
// confirmed_at the first time, and records the sign-in (Devise trackable).
// A wrong, expired or already-used code returns ok=false and no error.
func (s *Store) VerifyOTP(ctx context.Context, email, code, ip string) (User, bool, error) {
	var u User
	var stored string
	var fresh bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, COALESCE(api_token, ''), COALESCE(otp_code, ''),
		       COALESCE(otp_expires_at > `+nowUTC+`, false)
		FROM users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.APIToken, &stored, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	if stored == "" || !fresh || subtle.ConstantTimeCompare([]byte(stored), []byte(code)) != 1 {
		return User{}, false, nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET
		  otp_code = NULL, otp_expires_at = NULL,
		  confirmed_at = COALESCE(confirmed_at, `+nowUTC+`),
		  last_sign_in_at = COALESCE(current_sign_in_at, `+nowUTC+`),
		  last_sign_in_ip = COALESCE(current_sign_in_ip, $2),
		  current_sign_in_at = `+nowUTC+`,
		  current_sign_in_ip = $2,
		  sign_in_count = sign_in_count + 1,
		  updated_at = `+nowUTC+`
		WHERE id = $1 AND otp_code = $3`, u.ID, ip, stored)
	if err != nil {
		return User{}, false, err
	}
	return u, tag.RowsAffected() == 1, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, bool, error) {
	return s.userWhere(ctx, "id = $1", id)
}

func (s *Store) UserByAPIToken(ctx context.Context, token string) (User, bool, error) {
	return s.userWhere(ctx, "api_token = $1", token)
}

func (s *Store) userWhere(ctx context.Context, where string, arg any) (User, bool, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT id, email, COALESCE(api_token, '') FROM users WHERE `+where, arg).
		Scan(&u.ID, &u.Email, &u.APIToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}
