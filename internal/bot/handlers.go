package bot

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gopkg.in/telebot.v4"

	"vyaliksupport/internal/config"
	"vyaliksupport/internal/domain"
	"vyaliksupport/internal/listener"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"
)

// Bot holds all dependencies needed by the handlers.
type Bot struct {
	tb         *telebot.Bot
	cfg        config.Config
	repo       *postgres.Req
	ntfySender *sender.NtfySender
	lg         *zap.Logger
}

// New creates a Bot and registers all handlers.
func New(tb *telebot.Bot, cfg config.Config, repo *postgres.Req, ntfySender *sender.NtfySender, lg *zap.Logger) *Bot {
	b := &Bot{tb: tb, cfg: cfg, repo: repo, ntfySender: ntfySender, lg: lg}
	b.registerHandlers()
	return b
}

// Start runs the bot (blocking via go tb.Start with graceful stop on sigChan).
func (b *Bot) Start() {
	b.tb.Start()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	b.tb.Stop()
}

// HandleIncomingMessages processes messages received from ntfy (replies from group).
func (b *Bot) HandleIncomingMessages(ctx context.Context, ntfyListener *listener.NtfyListener) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-ntfyListener.Messages():
			if payload == nil {
				continue
			}
			if payload.Direction == domain.DirectionToUser {
				b.sendReplyToUser(payload)
			}
		}
	}
}

// sendReplyToUser sends a reply from support to the user.
func (b *Bot) sendReplyToUser(payload *domain.Payload) {
	userChatID := payload.UserChatID

	// Send the header message first so the user knows a reply is coming.
	if _, err := b.tb.Send(telebot.ChatID(userChatID), msgReplyHeader); err != nil {
		b.lg.Error("can't send reply header to user",
			zap.Int64("userChatID", userChatID),
			zap.Error(err),
		)
		return
	}

	// Send the content based on type.
	dst := telebot.ChatID(userChatID)

	switch payload.Content.Type {
	case domain.ContentTypeText:
		if payload.Content.Text != "" {
			_, err := b.tb.Send(dst, payload.Content.Text, &telebot.SendOptions{
				ParseMode: telebot.ModeMarkdown,
			})
			if err != nil {
				b.lg.Error("can't send text to user", zap.Error(err))
			}
		}

	case domain.ContentTypePhoto:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Photo{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				b.lg.Error("can't send photo to user", zap.Error(err))
			}
		}

	case domain.ContentTypeVideo:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Video{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				b.lg.Error("can't send video to user", zap.Error(err))
			}
		}

	case domain.ContentTypeDocument:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Document{
				File:     telebot.File{FileID: payload.Content.FileID},
				Caption:  payload.Content.Caption,
				FileName: payload.Content.FileName,
			})
			if err != nil {
				b.lg.Error("can't send document to user", zap.Error(err))
			}
		}

	case domain.ContentTypeSticker:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Sticker{File: telebot.File{FileID: payload.Content.FileID}})
			if err != nil {
				b.lg.Error("can't send sticker to user", zap.Error(err))
			}
		}

	case domain.ContentTypeAudio:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Audio{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				b.lg.Error("can't send audio to user", zap.Error(err))
			}
		}

	case domain.ContentTypeVoice:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Voice{File: telebot.File{FileID: payload.Content.FileID}})
			if err != nil {
				b.lg.Error("can't send voice to user", zap.Error(err))
			}
		}

	case domain.ContentTypeAnimation:
		if payload.Content.FileID != "" {
			_, err := b.tb.Send(dst, &telebot.Animation{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				b.lg.Error("can't send animation to user", zap.Error(err))
			}
		}
	}
}

// RunCleanup periodically removes expired requests.
func (b *Bot) RunCleanup(ctx context.Context, repo *postgres.Req, lg *zap.Logger) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := repo.Cleanup()
			if err != nil {
				lg.Error("cleanup error", zap.Error(err))
			} else if deleted > 0 {
				lg.Info("cleanup", zap.Int64("deleted", deleted))
			}
		}
	}
}

// registerHandlers wires up all telebot endpoints.
func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.handleStart)
	b.tb.Handle("/faq", b.handleFAQ)

	// FAQ inline button callbacks.
	// telebot v4 resolves a callback by Btn.CallbackUnique() == "\f" + Unique.
	// We pass a *Btn with only Unique set — Handle() only needs CallbackUnique(),
	// the ReplyMarkup instance is irrelevant for registration.
	for i := range faqItems {
		idx := i
		btn := &telebot.Btn{Unique: fmt.Sprintf("faq_%d", idx)}
		b.tb.Handle(btn, b.handleFAQAnswer(idx))
	}

	// "Back to FAQ" inline button callback.
	backBtn := &telebot.Btn{Unique: "faq_back"}
	b.tb.Handle(backBtn, b.handleFAQBack)

	// All media types route through the same dispatcher.
	for _, endpoint := range []interface{}{
		telebot.OnText,
		telebot.OnPhoto,
		telebot.OnVideo,
		telebot.OnDocument,
		telebot.OnSticker,
		telebot.OnAudio,
		telebot.OnVoice,
		telebot.OnAnimation,
	} {
		b.tb.Handle(endpoint, b.handleMessage)
	}
}

// handleStart sends the welcome message with the persistent reply keyboard.
// Uses ModeHTML because the welcome text contains links with underscores in usernames
// which would break Markdown parsing.
func (b *Bot) handleStart(c telebot.Context) error {
	msg := fmt.Sprintf(msgWelcome, b.cfg.VpnBot.URL, b.cfg.VpnBot.Name)
	return c.Send(msg, &telebot.SendOptions{
		ParseMode:   telebot.ModeHTML,
		ReplyMarkup: mainKeyboard(),
	})
}

// handleFAQ sends the FAQ inline menu.
func (b *Bot) handleFAQ(c telebot.Context) error {
	return c.Send(msgFAQ, &telebot.SendOptions{
		ParseMode:   telebot.ModeMarkdown,
		ReplyMarkup: b.faqKeyboard(),
	})
}

// handleFAQAnswer returns a callback handler that answers a specific FAQ item
// and appends a "back" inline button to return to the FAQ list.
func (b *Bot) handleFAQAnswer(idx int) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		// Acknowledge the button tap — removes the loading spinner.
		if err := c.Respond(); err != nil {
			b.lg.Warn("can't respond to callback", zap.Error(err))
		}
		return c.Send(faqItems[idx].Answer, &telebot.SendOptions{
			ParseMode:   telebot.ModeMarkdown,
			ReplyMarkup: backKeyboard(),
		})
	}
}

// handleFAQBack handles the inline "back" button — re-sends the FAQ menu.
func (b *Bot) handleFAQBack(c telebot.Context) error {
	if err := c.Respond(); err != nil {
		b.lg.Warn("can't respond to callback", zap.Error(err))
	}
	// Edit the current message to become the FAQ menu — no new message noise.
	return c.Edit(msgFAQ, &telebot.SendOptions{
		ParseMode:   telebot.ModeMarkdown,
		ReplyMarkup: b.faqKeyboard(),
	})
}

// handleMessage is the main dispatcher for all incoming messages.
// It routes based on context: persistent keyboard buttons, commands, or plain user message to forward.
func (b *Bot) handleMessage(c telebot.Context) error {
	msg := c.Message()

	// Ignore messages from the group (now handled via ntfy).
	if msg.Chat.ID == b.cfg.Bot.GroupID {
		return nil
	}

	switch msg.Text {
	case "/start", btnLabelHome:
		return b.handleStart(c)
	case "/faq", btnLabelFAQ:
		return b.handleFAQ(c)
	}

	// Regular user message — send to support group via ntfy.
	return b.handleUserMessage(c)
}

// handleUserMessage forwards a user's message to the support group via ntfy.
func (b *Bot) handleUserMessage(c telebot.Context) error {
	msg := c.Message()

	// Build the user summary card.
	summaryText := b.buildSummaryText(msg.Chat.ID)

	// Determine content type and extract file info.
	content := b.extractContent(msg)

	// Create payload.
	payload := &domain.Payload{
		Direction:  domain.DirectionToGroup,
		UserChatID: msg.Chat.ID,
		MsgID:      msg.ID,
		Summary:    summaryText,
		Content:    content,
		CreatedAt:  time.Now(),
	}

	// Send to ntfy.
	data, err := payload.Marshal()
	if err != nil {
		b.lg.Error("can't marshal payload", zap.Error(err))
		return c.Send("Не удалось отослать сообщение. Попробуйте ещё раз.")
	}

	if err := b.ntfySender.SendPayload(context.Background(), data); err != nil {
		b.lg.Error("can't send to ntfy", zap.Error(err))
		return c.Send("Не удалось отослать сообщение. Попробуйте ещё раз.")
	}

	// We need to track this message for replies.
	// Since we're not getting a real message ID from the group anymore,
	// we'll use the original message ID as a reference.
	// The group handler will store the user_chat_id -> original_msg_id mapping.
	// Actually, we need to save the user's message ID so the group can reference it.
	if err := b.repo.SaveRequest(int(msg.ID), msg.Chat.ID); err != nil {
		b.lg.Error("can't save support request", zap.Error(err))
	}

	return c.Send(msgSentToSupport)
}

// extractContent extracts content from a telebot.Message.
func (b *Bot) extractContent(msg *telebot.Message) domain.Content {
	content := domain.Content{Type: domain.ContentTypeText}

	switch {
	case msg.Text != "":
		content.Type = domain.ContentTypeText
		content.Text = msg.Text

	case msg.Photo != nil:
		content.Type = domain.ContentTypePhoto
		content.FileID = msg.Photo.File.FileID
		content.Caption = msg.Caption

	case msg.Video != nil:
		content.Type = domain.ContentTypeVideo
		content.FileID = msg.Video.File.FileID
		content.Caption = msg.Caption

	case msg.Document != nil:
		content.Type = domain.ContentTypeDocument
		content.FileID = msg.Document.File.FileID
		content.Caption = msg.Caption
		content.FileName = msg.Document.FileName

	case msg.Sticker != nil:
		content.Type = domain.ContentTypeSticker
		content.FileID = msg.Sticker.File.FileID

	case msg.Audio != nil:
		content.Type = domain.ContentTypeAudio
		content.FileID = msg.Audio.File.FileID
		content.Caption = msg.Caption

	case msg.Voice != nil:
		content.Type = domain.ContentTypeVoice
		content.FileID = msg.Voice.File.FileID

	case msg.Animation != nil:
		content.Type = domain.ContentTypeAnimation
		content.FileID = msg.Animation.File.FileID
		content.Caption = msg.Caption
	}

	return content
}

// buildSummaryText returns a formatted user card for the support group.
// Falls back to a plain header if the summary cannot be loaded.
func (b *Bot) buildSummaryText(chatID int64) string {
	summary, err := b.repo.GetUserSummary(chatID)
	if err != nil {
		b.lg.Error("can't get user summary", zap.Int64("tg_id", chatID), zap.Error(err))
		return fmt.Sprintf("💬 Новое сообщение от `%d`", chatID)
	}
	return "💬 *Новое обращение*\n\n" + summary.Format()
}

// faqKeyboard builds the inline keyboard for the FAQ menu.
// All buttons MUST be created from the same markup instance —
// telebot serialises InlineKeyboardMarkup from this object,
// and Unique in each Btn must match what was registered in Handle().
func (b *Bot) faqKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(faqItems))
	for i := range faqItems {
		btn := markup.Data(faqItems[i].Label, fmt.Sprintf("faq_%d", i))
		rows = append(rows, markup.Row(btn))
	}
	markup.Inline(rows...)
	return markup
}

// backKeyboard returns an inline keyboard with a single "back to FAQ" button.
func backKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	btn := markup.Data("⬅ Назад к FAQ", "faq_back")
	markup.Inline(markup.Row(btn))
	return markup
}

// mainKeyboard returns the persistent reply keyboard shown at the bottom of the chat.
// It never disappears until explicitly removed — always-on navigation for the user.
func mainKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{ResizeKeyboard: true}
	home := markup.Text(btnLabelHome)
	faq := markup.Text(btnLabelFAQ)
	markup.Reply(markup.Row(home, faq))
	return markup
}
