package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// defaultSlackClient bounds a hung webhook call when no Client is injected.
var defaultSlackClient = &http.Client{Timeout: 10 * time.Second}

// SlackNotifier posts OTP codes to a Slack incoming webhook. With no
// WebhookURL it does nothing (the code is still logged by the caller).
type SlackNotifier struct {
	WebhookURL string
	Client     *http.Client
}

func (n SlackNotifier) SendOTP(ctx context.Context, code string) error {
	if n.WebhookURL == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"text": "Your OTP for rulinky " + code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return redactURLError("build slack request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = defaultSlackClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return redactURLError("send slack request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack webhook answered %d", resp.StatusCode)
	}
	return nil
}

// redactURLError drops the webhook URL (a secret) that *url.Error embeds,
// keeping the operation and the underlying cause (still matchable with
// errors.Is, e.g. context.Canceled).
func redactURLError(what string, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %s: %w", what, ue.Op, ue.Err)
	}
	return fmt.Errorf("%s: %w", what, err)
}
