package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secretSegment = "T0SECRET/B1SECRET/xyzDISTINCTIVEtoken"

func assertNoSecret(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "DISTINCTIVE") || strings.Contains(err.Error(), "T0SECRET") {
		t.Fatalf("error leaks the webhook URL: %q", err)
	}
}

func TestSendOTPErrorHidesWebhookURLWhenUnreachable(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL + "/services/" + secretSegment
	ts.Close()
	err := SlackNotifier{WebhookURL: url}.SendOTP(context.Background(), "123456")
	assertNoSecret(t, err)
}

func TestSendOTPErrorHidesWebhookURLWhenCanceled(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SlackNotifier{WebhookURL: ts.URL + "/services/" + secretSegment}.SendOTP(ctx, "123456")
	assertNoSecret(t, err)
}

func TestSendOTPErrorHidesWebhookURLWhenMalformed(t *testing.T) {
	err := SlackNotifier{WebhookURL: "http://hooks.example/services/" + secretSegment + "\x7f"}.SendOTP(context.Background(), "123456")
	assertNoSecret(t, err)
}

func TestSendOTPErrorHidesWebhookURLOnBadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer ts.Close()
	err := SlackNotifier{WebhookURL: ts.URL + "/services/" + secretSegment}.SendOTP(context.Background(), "123456")
	assertNoSecret(t, err)
}
