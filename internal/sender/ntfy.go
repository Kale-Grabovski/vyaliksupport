package sender

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"vyaliksupport/internal/crypto"
)

type NtfySender struct {
	client      *http.Client
	baseURL     string
	topic       string
	token       string
	encryptKey  string
}

func NewNtfySender(topic, token, encryptKey string) *NtfySender {
	return &NtfySender{
		client:      &http.Client{Timeout: 5 * time.Second},
		baseURL:     "https://ntfy.sh",
		topic:       topic,
		token:       token,
		encryptKey:  encryptKey,
	}
}

// Send sends a plain text message.
func (s *NtfySender) Send(ctx context.Context, msg Message) error {
	return s.sendMessage(ctx, msg.Body, msg.Title, msg.Priority, msg.Tags, msg.Click, msg.Markdown)
}

// SendPayload sends an encrypted JSON payload.
func (s *NtfySender) SendPayload(ctx context.Context, payload []byte) error {
	body := base64.StdEncoding.EncodeToString(payload)
	if s.encryptKey != "" {
		encrypted, err := crypto.Encrypt(payload, s.encryptKey)
		if err != nil {
			return fmt.Errorf("encrypt payload: %w", err)
		}
		body = base64.StdEncoding.EncodeToString(encrypted)
	}
	return s.sendMessage(ctx, body, "", "", nil, "", false)
}

func (s *NtfySender) sendMessage(ctx context.Context, body, title, priority string, tags []string, click string, markdown bool) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/"+s.topic,
		strings.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build ntfy request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	if title != "" {
		req.Header.Set("Title", title)
	}
	if priority != "" {
		req.Header.Set("Priority", priority)
	}
	if len(tags) > 0 {
		req.Header.Set("Tags", strings.Join(tags, ","))
	}
	if click != "" {
		req.Header.Set("Click", click)
	}
	if markdown {
		req.Header.Set("Markdown", "yes")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ntfy bad status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return nil
}