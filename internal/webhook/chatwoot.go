package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"unicode/utf8"

	"vyaliksupport/internal/chatwoot"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"

	"go.uber.org/zap"
)

// ChatwootWebhook handles incoming webhooks from Chatwoot.
type ChatwootWebhook struct {
	woot   *chatwoot.Woot
	repo   *postgres.Req
	sender *sender.NtfySender
	lg     *zap.Logger
}

// NewChatwootWebhook creates a new Chatwoot webhook handler.
func NewChatwootWebhook(woot *chatwoot.Woot, repo *postgres.Req, lg *zap.Logger, sender *sender.NtfySender) *ChatwootWebhook {
	return &ChatwootWebhook{
		woot:   woot,
		repo:   repo,
		sender: sender,
		lg:     lg,
	}
}

// Start begins listening for webhooks on the specified address.
func (h *ChatwootWebhook) Start(listenAddr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.Handle)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	go func() {
		h.lg.Info("starting chatwoot webhook server", zap.String("addr", listenAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("webhook server failed: %v", err)
		}
	}()
}

// Handle processes incoming Chatwoot webhooks.
func (h *ChatwootWebhook) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event MessageCreatedEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		h.lg.Warn("failed to decode webhook", zap.Error(err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Only process incoming messages (from users)
	if event.MessageType != "incoming" || event.Event != "message_created" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get user summary from database
	summary, err := h.repo.GetUserSummary(event.Conversation.Attrs.TgID)
	if err != nil {
		h.lg.Error("failed to get user summary",
			zap.Int64("tg_id", event.Conversation.Attrs.TgID),
			zap.Error(err),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Build metadata note text
	metadata := "💬 *New message*\n\n" + summary.Format()

	// Send metadata note to Chatwoot
	if err := h.woot.SendMetadataNote(event.Account.ID, event.Conversation.ID, metadata); err != nil {
		h.lg.Error("failed to send metadata note",
			zap.Int("account_id", event.Account.ID),
			zap.Int("conv_id", event.Conversation.ID),
			zap.Error(err),
		)
	}

	h.lg.Info("received webhook",
		zap.Int("account_id", event.Account.ID),
		zap.Int("conv_id", event.Conversation.ID),
		zap.String("msg_type", event.MessageType),
	)

	h.notify(event.Conversation.Attrs.TgID, event.Sender.Attrs.Username, event.Content)

	w.WriteHeader(http.StatusOK)
}

func (h *ChatwootWebhook) notify(tgID int64, username, text string) {
	if utf8.RuneCountInString(text) > 70 {
		runes := []rune(text)
		text = string(runes[:70]) + "…"
	}
	ntfyMsg := sender.Message{
		Body:  text,
		Title: fmt.Sprintf("Message from %d %s", tgID, username),
		Tags:  []string{"incoming"},
	}
	if err := h.sender.Send(context.Background(), ntfyMsg); err != nil {
		h.lg.Warn("failed to send sender notification", zap.Error(err))
	}
}
