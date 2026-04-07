package listener

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"vyaliksupport/internal/crypto"
	"vyaliksupport/internal/domain"

	"go.uber.org/zap"
)

type ntfyMessage struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Event   string `json:"event"`
	Time    int64  `json:"time"`
}

type NtfyListener struct {
	client      *http.Client
	baseURL     string
	topic       string
	token       string
	encryptKey  string
	lg          *zap.Logger
	mu          sync.Mutex
	running     bool
	stopChan    chan struct{}
	messageChan chan *domain.Payload
}

func NewNtfyListener(topic, token, encryptKey string, lg *zap.Logger) *NtfyListener {
	return &NtfyListener{
		client:      &http.Client{Timeout: 30 * time.Second},
		baseURL:     "https://ntfy.sh",
		topic:       topic,
		token:       token,
		encryptKey:  encryptKey,
		lg:          lg,
		stopChan:    make(chan struct{}),
		messageChan: make(chan *domain.Payload, 100),
	}
}

// Messages returns a channel that receives decrypted payloads.
func (l *NtfyListener) Messages() <-chan *domain.Payload {
	return l.messageChan
}

// Start begins listening for messages from the ntfy topic.
func (l *NtfyListener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return fmt.Errorf("listener already running")
	}
	l.running = true
	l.mu.Unlock()

	go l.poll(ctx)
	return nil
}

// Stop stops the listener.
func (l *NtfyListener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return
	}
	l.running = false
	close(l.stopChan)
	l.stopChan = make(chan struct{})
}

func (l *NtfyListener) poll(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		default:
			// reconnect on error with backoff
			if err := l.stream(ctx); err != nil {
				l.lg.Error("ntfy stream error, reconnecting", zap.Error(err))
				select {
				case <-time.After(5 * time.Second):
				case <-ctx.Done():
					return
				case <-l.stopChan:
					return
				}
			}
		}
	}
}

func (l *NtfyListener) stream(ctx context.Context) error {
	// Use ?poll=1 for one-shot or just /json for a persistent stream.
	// Add since=all or since=<unix_timestamp> to avoid replaying old messages!
	url := fmt.Sprintf("%s/%s/json?since=%d", l.baseURL, l.topic, time.Now().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
	}

	// NO timeout on the client for streaming — use context cancellation instead
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ntfy bad status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		// Check for cancellation between messages
		select {
		case <-ctx.Done():
			return nil
		case <-l.stopChan:
			return nil
		default:
		}

		var msg ntfyMessage
		if err := decoder.Decode(&msg); err != nil {
			// EOF means server closed connection — reconnect
			return fmt.Errorf("stream decode: %w", err)
		}

		if msg.Event == "message" {
			l.processMessage(&msg)
		}
		// "open" and "keepalive" events are silently ignored
	}
}

func (l *NtfyListener) processMessage(msg *ntfyMessage) {
	if msg.Message == "" {
		return
	}

	// Decrypt the message if encryption key is provided.
	var payload *domain.Payload
	var err error

	if l.encryptKey != "" {
		ciphertext, err := base64.StdEncoding.DecodeString(msg.Message)
		if err != nil {
			l.lg.Error("failed to decode base64 message", zap.Error(err))
			return
		}

		plaintext, err := crypto.Decrypt(ciphertext, l.encryptKey)
		if err != nil {
			l.lg.Error("failed to decrypt message", zap.Error(err))
			return
		}

		payload, err = domain.UnmarshalPayload(plaintext)
		if err != nil {
			l.lg.Error("failed to unmarshal payload", zap.Error(err))
			return
		}
	} else {
		// No encryption - try to unmarshal directly.
		payload, err = domain.UnmarshalPayload([]byte(msg.Message))
		if err != nil {
			l.lg.Error("failed to unmarshal payload (no encryption)", zap.Error(err))
			return
		}
	}

	select {
	case l.messageChan <- payload:
	default:
		l.lg.Warn("message channel full, dropping message")
	}
}

func (l *NtfyListener) stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running {
		l.running = false
		close(l.messageChan)
		l.messageChan = make(chan *domain.Payload, 100)
	}
}
