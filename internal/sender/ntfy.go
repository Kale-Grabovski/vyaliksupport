package sender

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type NtfySender struct {
	client  *http.Client
	baseURL string
	topic   string
	token   string
}

func NewNtfySender(topic, token string) *NtfySender {
	return &NtfySender{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: "https://ntfy.sh",
		topic:   topic,
		token:   token,
	}
}

func (s *NtfySender) Send(ctx context.Context, msg Message) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/"+s.topic,
		strings.NewReader(msg.Body),
	)
	if err != nil {
		return fmt.Errorf("build ntfy request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	if msg.Title != "" {
		req.Header.Set("Title", msg.Title)
	}
	if msg.Priority != "" {
		req.Header.Set("Priority", msg.Priority)
	}
	if len(msg.Tags) > 0 {
		req.Header.Set("Tags", strings.Join(msg.Tags, ","))
	}
	if msg.Click != "" {
		req.Header.Set("Click", msg.Click)
	}
	if msg.Markdown {
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
