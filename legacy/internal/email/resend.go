package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ResendMailer sends email via the Resend HTTP API.
type ResendMailer struct {
	apiKey string
	from   string
}

// NewResendMailer creates a Resend-backed mailer.
func NewResendMailer(apiKey, from string) *ResendMailer {
	return &ResendMailer{apiKey: apiKey, from: from}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

type resendError struct {
	Message string `json:"message"`
}

func (m *ResendMailer) Send(ctx context.Context, msg *Message) error {
	from := msg.From
	if from == "" {
		from = m.from
	}

	body := resendRequest{
		From:    from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("resend API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
