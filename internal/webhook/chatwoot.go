package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"vyaliksupport/internal/chatwoot"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"

	"go.uber.org/zap"
	"gopkg.in/telebot.v4"
)

const msgReplyHeader = `👨‍💻 <b>Ответ от поддержки:</b>`

// BotPusher is implemented by *telebot.Bot.
type BotPusher interface {
	Send(to telebot.Recipient, what interface{}, options ...interface{}) (*telebot.Message, error)
}

// ChatwootWebhook handles incoming webhooks from Chatwoot.
type ChatwootWebhook struct {
	woot   *chatwoot.Woot
	repo   *postgres.Req
	sender *sender.NtfySender
	lg     *zap.Logger
	bot    BotPusher // injected via SetBot
}

// SetBot injects the Telegram bot after it is created.
func (h *ChatwootWebhook) SetBot(bot BotPusher) {
	h.bot = bot
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal", http.StatusInternalServerError)
		return
	}

	var event MessageCreatedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		h.lg.Warn("failed to decode webhook", zap.Error(err))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Only process incoming messages (from users)
	if !strings.Contains(event.Event, "message_created") {
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.MessageType != "incoming" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get user summary from database
	//tgID := event.Meta.Sender.AdditionalAttributes.SocialTelegramUserId
	//summary, err := h.repo.GetUserSummary(tgID)
	//if err != nil {
	//	h.lg.Error("failed to get user summary",
	//		zap.Int64("tg_id", tgID),
	//		zap.Error(err),
	//	)
	//	w.WriteHeader(http.StatusOK)
	//	return
	//}

	// Build metadata note text
	//metadata := "💬 *New message*\n\n" + summary.Format()

	// Send metadata note to Chatwoot
	//if err := h.woot.SendMetadataNote(msg.AccountId, msg.ConversationId, metadata); err != nil {
	//	h.lg.Error("failed to send metadata note",
	//		zap.Int("account_id", msg.AccountId),
	//		zap.Int("conv_id", msg.ConversationId),
	//		zap.Error(err),
	//	)
	//}
	//
	//h.lg.Info("received webhook",
	//	zap.Int("account_id", msg.AccountId),
	//	zap.Int("conv_id", msg.ConversationId),
	//	zap.Int("msg_type", msg.MessageType),
	//)

	//h.notify(tgID, event.Meta.Sender.AdditionalAttributes.Username, msg.Content)

	w.WriteHeader(http.StatusOK)
}

func (h *ChatwootWebhook) notify(tgID int64, username, text string) {
	// Forward the agent's reply to the Telegram user.
	if h.bot != nil {
		tgUser := &telebot.User{ID: tgID}
		_, err := h.bot.Send(tgUser, msgReplyHeader+"\n\n"+text, &telebot.SendOptions{
			ParseMode: telebot.ModeHTML,
		})
		if err != nil {
			h.lg.Warn("failed to send reply to Telegram user",
				zap.Int64("tg_id", tgID),
				zap.Error(err),
			)
		} else {
			h.lg.Info("forwarded agent reply to Telegram user", zap.Int64("tg_id", tgID))
		}
	}

	// Also push a notification via ntfy (for operators who watch that channel).
	displayText := text
	if utf8.RuneCountInString(text) > 70 {
		displayText = string([]rune(text)[:70]) + "…"
	}
	ntfyMsg := sender.Message{
		Body:  displayText,
		Title: fmt.Sprintf("Reply to %d %s", tgID, username),
		Tags:  []string{"outgoing"},
	}
	if err := h.sender.Send(context.Background(), ntfyMsg); err != nil {
		h.lg.Warn("failed to send ntfy notification", zap.Error(err))
	}
}
