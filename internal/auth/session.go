// Package auth holds the session cookie, OTP generation and OTP delivery.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	CookieName = "_rulinky_session"
	// Timeout is the inactivity limit (Devise timeoutable was 10 days).
	Timeout = 10 * 24 * time.Hour
)

// Session is what the signed cookie carries. UserID is set once signed in;
// AuthEmail is the address waiting for its OTP.
type Session struct {
	UserID    int64  `json:"u,omitempty"`
	AuthEmail string `json:"e,omitempty"`
	LastSeen  int64  `json:"t"`
}

type Sessions struct {
	secret []byte
	secure bool
	now    func() time.Time
}

func NewSessions(secret []byte, secure bool) *Sessions {
	return &Sessions{secret: secret, secure: secure, now: time.Now}
}

// Read returns the request's session, or a zero Session when the cookie is
// missing, forged, or older than Timeout since its last write.
func (s *Sessions) Read(r *http.Request) Session {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return Session{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}
	}
	var sess Session
	if json.Unmarshal(raw, &sess) != nil {
		return Session{}
	}
	if s.now().Sub(time.Unix(sess.LastSeen, 0)) > Timeout {
		return Session{}
	}
	return sess
}

// Write stores sess (stamped with the current time) in the cookie. Call it
// before writing the response body.
func (s *Sessions) Write(w http.ResponseWriter, sess Session) {
	sess.LastSeen = s.now().Unix()
	raw, _ := json.Marshal(sess)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, s.cookie(payload+"."+s.sign(payload), int(Timeout/time.Second)))
}

func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, s.cookie("", -1))
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	}
}

func (s *Sessions) sign(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
