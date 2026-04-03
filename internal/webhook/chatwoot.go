package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"vyaliksupport/internal/chatwoot"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"

	"go.uber.org/zap"
	"gopkg.in/telebot.v4"
)

const msgReplyHeader = `👨‍💻 <b>Ответ от поддержки:</b>`

const (
	btnLabelHome = "🏠 Главная"
	btnLabelFAQ  = "❓ FAQ"
)

// mainKeyboard returns the persistent reply keyboard shown at the bottom of the chat.
// This preserves the quick reply buttons for the user.
func mainKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{ResizeKeyboard: true}
	home := markup.Text(btnLabelHome)
	faq := markup.Text(btnLabelFAQ)
	markup.Reply(markup.Row(home, faq))
	return markup
}

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
	cfg    WebhookConfig
}

// WebhookConfig holds configuration for the webhook.
type WebhookConfig struct {
	TelegramBotToken string // For downloading files from Telegram
}

// SetBot injects the Telegram bot after it is created.
func (h *ChatwootWebhook) SetBot(bot BotPusher) {
	h.bot = bot
}

// SetConfig sets webhook configuration.
func (h *ChatwootWebhook) SetConfig(cfg WebhookConfig) {
	h.cfg = cfg
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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

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
		h.lg.Warn("failed to decode webhook (trying AutoMessageCreatedEvent)", zap.Error(err))
		// Try alternative format
		var autoEvent AutoMessageCreatedEvent
		if err2 := json.Unmarshal(body, &autoEvent); err2 == nil {
			h.handleAutoMessageEvent(&autoEvent)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	h.handleMessageEvent(&event)
	w.WriteHeader(http.StatusOK)
}

// handleMessageEvent processes a standard message_created event.
func (h *ChatwootWebhook) handleMessageEvent(event *MessageCreatedEvent) {
	// Only process incoming messages (from users)
	if !strings.Contains(event.Event, "message_created") {
		return
	}

	if event.MessageType != "incoming" {
		return
	}

	// Get Telegram user ID from conversation
	tgID := h.extractTelegramID(event)
	if tgID == 0 {
		h.lg.Warn("could not extract Telegram ID from event", zap.Any("event", event))
		return
	}

	// Forward the message to Telegram user
	username := event.Sender.Attrs.Username
	h.forwardToTelegram(tgID, username, event.Content, event.Attachments)
}

// handleAutoMessageEvent processes events from Chatwoot automation.
func (h *ChatwootWebhook) handleAutoMessageEvent(event *AutoMessageCreatedEvent) {
	if len(event.Messages) == 0 {
		return
	}

	// Get the message
	msg := event.Messages[0]

	// Skip private notes
	if msg.Private {
		return
	}

	// Get Telegram ID from various sources
	var tgID int64

	// Try from meta sender additional attributes
	tgID = event.Meta.Sender.AdditionalAttributes.SocialTelegramUserId
	if tgID == 0 {
		// Try from additional attributes
		tgID = int64(event.AdditionalAttributes.ChatId)
	}
	if tgID == 0 {
		// Try from conversation identifier via API
		tgIDFromConv, err := h.getTelegramIDFromChatwoot(event.Messages[0].Conversation.AssigneeId, event.Messages[0].ConversationId)
		if err == nil {
			tgID = tgIDFromConv
		}
	}

	if tgID == 0 {
		h.lg.Warn("could not extract Telegram ID from auto event",
			zap.Any("event_type", event.Channel),
			zap.Int64("social_telegram_id", event.Meta.Sender.AdditionalAttributes.SocialTelegramUserId),
			zap.Int("chat_id", event.AdditionalAttributes.ChatId),
		)
		return
	}

	username := event.Meta.Sender.AdditionalAttributes.SocialTelegramUserName
	if username == "" {
		username = event.Meta.Sender.Name
	}

	// Forward to Telegram
	attachments := msg.Attachments
	h.forwardToTelegram(tgID, username, msg.Content, attachments)
}

// extractTelegramID tries to get Telegram user ID from various places.
func (h *ChatwootWebhook) extractTelegramID(event *MessageCreatedEvent) int64 {
	// Try from conversation additional_attributes (custom_chat_id field in Chatwoot)
	if event.Conversation.Attrs.TgID != 0 {
		return event.Conversation.Attrs.TgID
	}

	// Try to get from Chatwoot API using conversation ID
	tgID, err := h.getTelegramIDFromChatwoot(event.Account.ID, event.Conversation.ID)
	if err != nil {
		h.lg.Warn("failed to get Telegram ID from Chatwoot API", zap.Error(err))
	}
	if tgID != 0 {
		return tgID
	}

	// Try from sender identifier (contact identifier like "tg:123456")
	return 0
}

// getTelegramIDFromChatwoot fetches the conversation from Chatwoot and extracts Telegram ID.
func (h *ChatwootWebhook) getTelegramIDFromChatwoot(accountID, convID int) (int64, error) {
	info, err := h.woot.GetConversationInfo(accountID, convID)
	if err != nil {
		return 0, err
	}

	// Extract from identifier (format: "tg:123456789")
	if strings.HasPrefix(info.Identifier, "tg:") {
		idStr := strings.TrimPrefix(info.Identifier, "tg:")
		tgID, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			return tgID, nil
		}
	}

	// Try parsing as plain number
	if info.Identifier != "" {
		tgID, err := strconv.ParseInt(info.Identifier, 10, 64)
		if err == nil && tgID > 0 {
			return tgID, nil
		}
	}

	return 0, fmt.Errorf("Telegram ID not found in conversation")
}

// forwardToTelegram sends the message content to a Telegram user.
func (h *ChatwootWebhook) forwardToTelegram(tgID int64, username, text string, attachments []Attachment) {
	if h.bot == nil {
		h.lg.Warn("bot not set, cannot forward message")
		return
	}

	tgUser := &telebot.User{ID: tgID}

	// Build message content
	content := text
	if content == "" && len(attachments) == 0 {
		content = "[empty message]"
	}

	// If there are attachments, download and forward them
	if len(attachments) > 0 {
		h.sendMediaToTelegram(tgUser, content, attachments)
		h.notify(tgID, username, content)
		return
	}

	// Plain text message with reply keyboard preserved
	_, err := h.bot.Send(tgUser, msgReplyHeader+"\n\n"+content, &telebot.SendOptions{
		ParseMode:   telebot.ModeHTML,
		ReplyMarkup: mainKeyboard(),
	})
	if err != nil {
		h.lg.Warn("failed to send reply to Telegram user",
			zap.Int64("tg_id", tgID),
			zap.Error(err),
		)
	} else {
		h.lg.Info("forwarded agent reply to Telegram user", zap.Int64("tg_id", tgID))
	}

	// Also push ntfy notification
	h.notify(tgID, username, content)
}

// sendMediaToTelegram handles sending messages with attachments to Telegram.
func (h *ChatwootWebhook) sendMediaToTelegram(tgUser *telebot.User, text string, attachments []Attachment) {
	if len(attachments) == 0 {
		if text != "" {
			h.bot.Send(tgUser, msgReplyHeader+"\n\n"+text, &telebot.SendOptions{
				ParseMode: telebot.ModeHTML,
			})
		}
		return
	}

	// Build the header text once
	captionText := text
	if captionText != "" {
		captionText = msgReplyHeader + "\n\n" + text
	}

	// Send first attachment with caption
	att := attachments[0]
	if att.URL != "" {
		fileData, contentType, err := downloadFile(att.URL)
		if err != nil {
			h.lg.Warn("failed to download first attachment",
				zap.String("url", att.URL),
				zap.Error(err),
			)
			// Fallback to text
			h.bot.Send(tgUser, msgReplyHeader+"\n\n"+text, &telebot.SendOptions{
				ParseMode: telebot.ModeHTML,
			})
		} else {
			ext := extFromContentType(contentType, att.FileName)
			filename := att.FileName
			if filename == "" {
				filename = "file" + ext
			}
			h.sendFileToTelegram(tgUser, captionText, fileData, filename, contentType, true)
		}
	} else {
		h.bot.Send(tgUser, msgReplyHeader+"\n\n"+text, &telebot.SendOptions{
			ParseMode: telebot.ModeHTML,
		})
	}

	// Send remaining attachments without caption (or with minimal text)
	for i := 1; i < len(attachments); i++ {
		att := attachments[i]
		if att.URL == "" {
			continue
		}

		fileData, contentType, err := downloadFile(att.URL)
		if err != nil {
			h.lg.Warn("failed to download attachment",
				zap.String("url", att.URL),
				zap.Error(err),
			)
			continue
		}

		ext := extFromContentType(contentType, att.FileName)
		filename := att.FileName
		if filename == "" {
			filename = "file" + ext
		}

		h.sendFileToTelegram(tgUser, "", fileData, filename, contentType, false)
	}
}

// sendFileToTelegram sends a file with optional caption to a Telegram user.
func (h *ChatwootWebhook) sendFileToTelegram(tgUser *telebot.User, caption string, data []byte, filename, contentType string, withCaption bool) error {
	reader := bytes.NewReader(data)
	mediaType := detectTelegramMediaType(contentType)

	var opts []interface{}
	if withCaption && caption != "" {
		opts = append(opts, &telebot.SendOptions{
			ParseMode:   telebot.ModeHTML,
			ReplyMarkup: mainKeyboard(),
		})
	} else {
		opts = append(opts, &telebot.SendOptions{
			ReplyMarkup: mainKeyboard(),
		})
	}

	switch mediaType {
	case "photo":
		photo := &telebot.Photo{
			File:    telebot.FromReader(reader),
			Caption: caption,
		}
		_, err := h.bot.Send(tgUser, photo, opts...)
		return err

	case "video":
		video := &telebot.Video{
			File:    telebot.FromReader(reader),
			Caption: caption,
		}
		_, err := h.bot.Send(tgUser, video, opts...)
		return err

	case "document":
		doc := &telebot.Document{
			File:    telebot.FromReader(reader),
			Caption: caption,
		}
		_, err := h.bot.Send(tgUser, doc, opts...)
		return err

	case "audio":
		audio := &telebot.Audio{
			File:    telebot.FromReader(reader),
			Caption: caption,
		}
		_, err := h.bot.Send(tgUser, audio, opts...)
		return err

	case "voice":
		voice := &telebot.Voice{
			File: telebot.FromReader(reader),
		}
		_, err := h.bot.Send(tgUser, voice, opts...)
		return err

	default:
		doc := &telebot.Document{
			File:    telebot.FromReader(reader),
			Caption: caption,
		}
		_, err := h.bot.Send(tgUser, doc, opts...)
		return err
	}
}

// downloadFile downloads a file from URL and returns data + content type.
func downloadFile(url string) ([]byte, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return data, contentType, nil
}

// detectTelegramMediaType determines the Telegram media type from content type.
func detectTelegramMediaType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	if strings.HasPrefix(contentType, "image/") {
		return "photo"
	}
	if strings.HasPrefix(contentType, "video/") {
		return "video"
	}
	if strings.HasPrefix(contentType, "audio/") {
		if contentType == "audio/ogg" || contentType == "audio/opus" {
			return "voice"
		}
		return "audio"
	}
	if contentType == "application/pdf" || contentType == "application/octet-stream" {
		return "document"
	}
	if strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "document") ||
		strings.Contains(contentType, "word") ||
		strings.Contains(contentType, "excel") ||
		strings.Contains(contentType, "pdf") ||
		strings.Contains(contentType, "zip") ||
		strings.Contains(contentType, "archive") {
		return "document"
	}

	return "document"
}

// extFromContentType determines file extension from content type.
func extFromContentType(contentType, filename string) string {
	// First try to get from provided filename
	if filename != "" {
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != "" {
			return ext
		}
	}

	// Fall back to content type
	contentType = strings.ToLower(contentType)

	extMap := map[string]string{
		"image/jpeg":               ".jpg",
		"image/png":                ".png",
		"image/gif":                ".gif",
		"image/webp":               ".webp",
		"image/svg+xml":            ".svg",
		"video/mp4":                ".mp4",
		"video/quicktime":          ".mov",
		"video/x-msvideo":          ".avi",
		"video/webm":               ".webm",
		"audio/mpeg":               ".mp3",
		"audio/ogg":                ".ogg",
		"audio/opuse":              ".opus",
		"audio/wav":                ".wav",
		"audio/webm":               ".webm",
		"application/pdf":          ".pdf",
		"application/octet-stream": ".bin",
		"text/plain":               ".txt",
		"application/msword":       ".doc",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
		"application/zip": ".zip",
	}

	if ext, ok := extMap[contentType]; ok {
		return ext
	}
	return ".bin"
}

func (h *ChatwootWebhook) notify(tgID int64, username, text string) {
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
