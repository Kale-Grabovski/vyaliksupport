package webhook

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"vyaliksupport/internal/chatwoot"
	"vyaliksupport/pkg/db/postgres"

	"go.uber.org/zap"
)

// ChatwootWebhook handles incoming webhooks from Chatwoot.
type ChatwootWebhook struct {
	woot *chatwoot.Woot
	repo *postgres.Req
	lg   *zap.Logger
}

// MessageCreatedEvent represents the webhook payload for message_created event.
type MessageCreatedEvent struct {
	Event        string `json:"event"`
	ID           int    `json:"id"`
	Content      string `json:"content"`
	MessageType  string `json:"message_type"`
	Conversation struct {
		ID      int `json:"id"`
		InboxID int `json:"inbox_id"`
		Attrs   struct {
			TgID int64 `json:"chat_id"`
		} `json:"additional_attributes"`
	} `json:"conversation"`
	Account struct {
		ID int `json:"id"`
	} `json:"account"`
}

// NewChatwootWebhook creates a new Chatwoot webhook handler.
func NewChatwootWebhook(woot *chatwoot.Woot, repo *postgres.Req, lg *zap.Logger) *ChatwootWebhook {
	return &ChatwootWebhook{
		woot: woot,
		repo: repo,
		lg:   lg,
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

	w.WriteHeader(http.StatusOK)
}
