package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func roundTrip(s *Sessions, sess Session) *http.Request {
	rec := httptest.NewRecorder()
	s.Write(rec, sess)
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestSessionRoundTrip(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), true)
	got := s.Read(roundTrip(s, Session{UserID: 7, AuthEmail: "a@b.c"}))
	if got.UserID != 7 || got.AuthEmail != "a@b.c" {
		t.Fatalf("got %+v", got)
	}

	rec := httptest.NewRecorder()
	s.Write(rec, Session{UserID: 7})
	c := rec.Result().Cookies()[0]
	if c.Name != CookieName || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(Timeout/time.Second) {
		t.Fatalf("cookie flags wrong: %+v", c)
	}
}

func TestSessionRejectsTamperingAndOtherSecrets(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	req := roundTrip(s, Session{UserID: 7})
	c, _ := req.Cookie(CookieName)

	forged := httptest.NewRequest("GET", "/", nil)
	forged.AddCookie(&http.Cookie{Name: CookieName, Value: "x" + c.Value})
	if got := s.Read(forged); got.UserID != 0 {
		t.Fatal("tampered cookie accepted")
	}
	other := NewSessions([]byte(strings.Repeat("z", 32)), false)
	if got := other.Read(req); got.UserID != 0 {
		t.Fatal("cookie signed with another secret accepted")
	}
}

func TestSessionExpiresAfterInactivity(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	start := time.Now()
	s.now = func() time.Time { return start }
	req := roundTrip(s, Session{UserID: 7})

	s.now = func() time.Time { return start.Add(Timeout - time.Minute) }
	if s.Read(req).UserID != 7 {
		t.Fatal("expired too early")
	}
	s.now = func() time.Time { return start.Add(Timeout + time.Minute) }
	if s.Read(req).UserID != 0 {
		t.Fatal("not expired after the timeout")
	}
}

func TestSessionClear(t *testing.T) {
	s := NewSessions([]byte(strings.Repeat("k", 32)), false)
	rec := httptest.NewRecorder()
	s.Clear(rec)
	if c := rec.Result().Cookies()[0]; c.Name != CookieName || c.MaxAge >= 0 {
		t.Fatalf("clear cookie wrong: %+v", c)
	}
}

func TestNewOTP(t *testing.T) {
	re := regexp.MustCompile(`^\d{6}$`)
	for range 50 {
		code, err := NewOTP()
		if err != nil || !re.MatchString(code) {
			t.Fatalf("code %q err %v", code, err)
		}
	}
}

func TestValidEmail(t *testing.T) {
	for _, e := range []string{"me@example.com", "a.b+c@sub.example.co"} {
		if !ValidEmail(e) {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range []string{"", "me", "me@", "@example.com", "me@exa mple.com", "me@-x.com"} {
		if ValidEmail(e) {
			t.Errorf("%q should be invalid", e)
		}
	}
}

func TestSlackSendOTP(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type %q", r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()

	n := SlackNotifier{WebhookURL: srv.URL, Client: srv.Client()}
	if err := n.SendOTP(context.Background(), "123456"); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "Your OTP for rulinky 123456" {
		t.Fatalf("payload %v", got)
	}
}

func TestSlackSkipsWithoutWebhookAndReportsErrors(t *testing.T) {
	if err := (SlackNotifier{}).SendOTP(context.Background(), "1"); err != nil {
		t.Fatalf("empty webhook must be a no-op, got %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := (SlackNotifier{WebhookURL: srv.URL, Client: srv.Client()}).SendOTP(context.Background(), "1"); err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
}

func TestSlackSendOTPHonorsContextDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- (SlackNotifier{WebhookURL: srv.URL}).SendOTP(ctx, "123456")
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a blocked webhook")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendOTP hung past its context deadline")
	}
}
