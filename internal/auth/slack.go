package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack webhook answered %d", resp.StatusCode)
	}
	return nil
}
