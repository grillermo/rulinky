package store

import (
	"context"
	"testing"
)

func TestFindOrCreateUserByEmail(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()

	u1, err := st.FindOrCreateUserByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID == 0 || u1.Email != "a@example.com" || len(u1.APIToken) != 24 {
		t.Fatalf("unexpected user %+v", u1)
	}
	u2, err := st.FindOrCreateUserByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID || u2.APIToken != u1.APIToken {
		t.Fatalf("second call must return the same user: %+v vs %+v", u2, u1)
	}
}

func TestFindOrCreateFillsMissingAPIToken(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	if _, err := st.Pool().Exec(ctx, `INSERT INTO users (email, created_at, updated_at) VALUES ('old@example.com', now(), now())`); err != nil {
		t.Fatal(err)
	}
	u, err := st.FindOrCreateUserByEmail(ctx, "old@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.APIToken) != 24 {
		t.Fatalf("token not filled: %q", u.APIToken)
	}
}

func TestVerifyOTP(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	if err := st.SetOTP(ctx, u.ID, "012345"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := st.VerifyOTP(ctx, "me@example.com", "999999", "1.2.3.4"); err != nil || ok {
		t.Fatalf("wrong code accepted: ok=%v err=%v", ok, err)
	}
	got, ok, err := st.VerifyOTP(ctx, "me@example.com", "012345", "1.2.3.4")
	if err != nil || !ok || got.ID != u.ID {
		t.Fatalf("right code rejected: ok=%v err=%v user=%+v", ok, err, got)
	}
	if _, ok, _ := st.VerifyOTP(ctx, "me@example.com", "012345", "1.2.3.4"); ok {
		t.Fatal("a code must work only once")
	}

	var count int
	var ip string
	var confirmed, otpNull bool
	err = st.Pool().QueryRow(ctx, `SELECT sign_in_count, current_sign_in_ip, confirmed_at IS NOT NULL, otp_code IS NULL FROM users WHERE id = $1`, u.ID).
		Scan(&count, &ip, &confirmed, &otpNull)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || ip != "1.2.3.4" || !confirmed || !otpNull {
		t.Fatalf("trackable/confirm not updated: count=%d ip=%q confirmed=%v otpNull=%v", count, ip, confirmed, otpNull)
	}
}

func TestVerifyOTPExpired(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")
	_ = st.SetOTP(ctx, u.ID, "123456")
	if _, err := st.Pool().Exec(ctx, `UPDATE users SET otp_expires_at = `+nowUTC+` - interval '1 minute' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.VerifyOTP(ctx, "me@example.com", "123456", ""); ok {
		t.Fatal("expired code accepted")
	}
}

func TestVerifyOTPUnknownEmail(t *testing.T) {
	st := OpenTest(t)
	if _, ok, err := st.VerifyOTP(context.Background(), "nobody@example.com", "123456", ""); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestUserLookups(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u, _ := st.FindOrCreateUserByEmail(ctx, "me@example.com")

	byID, ok, err := st.UserByID(ctx, u.ID)
	if err != nil || !ok || byID != u {
		t.Fatalf("UserByID: %+v ok=%v err=%v", byID, ok, err)
	}
	byToken, ok, err := st.UserByAPIToken(ctx, u.APIToken)
	if err != nil || !ok || byToken != u {
		t.Fatalf("UserByAPIToken: %+v ok=%v err=%v", byToken, ok, err)
	}
	if _, ok, _ := st.UserByAPIToken(ctx, "nope"); ok {
		t.Fatal("unknown token found a user")
	}
	if _, ok, _ := st.UserByID(ctx, 999999); ok {
		t.Fatal("unknown id found a user")
	}
}
