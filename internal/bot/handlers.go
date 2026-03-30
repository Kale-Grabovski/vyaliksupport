package bot

import (
	"fmt"

	"go.uber.org/zap"
	"gopkg.in/telebot.v4"

	"vyaliksupport/internal/config"
	"vyaliksupport/pkg/db/postgres"
)

// Bot holds all dependencies needed by the handlers.
type Bot struct {
	tb   *telebot.Bot
	cfg  config.Config
	repo *postgres.Req
	lg   *zap.Logger
}

// New creates a Bot and registers all handlers.
func New(tb *telebot.Bot, cfg config.Config, repo *postgres.Req, lg *zap.Logger) *Bot {
	b := &Bot{tb: tb, cfg: cfg, repo: repo, lg: lg}
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
// It routes based on context: group reply, persistent keyboard buttons,
// commands, or plain user message to forward.
func (b *Bot) handleMessage(c telebot.Context) error {
	msg := c.Message()

	// Support group: operator is replying to a forwarded message.
	if msg.Chat.ID == b.cfg.Bot.GroupID && msg.ReplyTo != nil {
		return b.handleGroupReply(c)
	}

	if msg.Chat.ID != b.cfg.Bot.GroupID {
		switch msg.Text {
		case "/start", btnLabelHome:
			return b.handleStart(c)
		case "/faq", btnLabelFAQ:
			return b.handleFAQ(c)
		}

		// Regular user message — forward to support group.
		return b.handleUserMessage(c)
	}

	return nil
}

// handleGroupReply forwards an operator's reply back to the original user.
func (b *Bot) handleGroupReply(c telebot.Context) error {
	msg := c.Message()

	userChatID, err := b.repo.FindUserChatID(msg.ReplyTo.ID)
	if err != nil {
		b.sendToGroup(msgReplyNotFound)
		return nil
	}

	// Send the header message first so the user knows a reply is coming.
	if _, err = b.tb.Send(telebot.ChatID(userChatID), msgReplyHeader); err != nil {
		b.lg.Error("can't send reply header to user",
			zap.Int64("userChatID", userChatID),
			zap.Error(err),
		)
		b.sendToGroup(msgReplyUserBlocked)
		return nil
	}

	// Forward the actual content depending on media type.
	if err = b.forwardContentToUser(userChatID, msg); err != nil {
		b.lg.Error("can't forward content to user",
			zap.Int64("userChatID", userChatID),
			zap.Error(err),
		)
		b.sendToGroup(msgReplyUserBlocked)
		return nil
	}

	b.sendToGroup(msgReplySentOK)
	return nil
}

// forwardContentToUser sends the correct media type to the user.
func (b *Bot) forwardContentToUser(userChatID int64, msg *telebot.Message) error {
	dst := telebot.ChatID(userChatID)

	switch {
	case msg.Text != "":
		_, err := b.tb.Send(dst, msg.Text, &telebot.SendOptions{
			ParseMode: telebot.ModeMarkdown,
		})
		return err

	case msg.Photo != nil:
		_, err := b.tb.Send(dst, &telebot.Photo{File: msg.Photo.File, Caption: msg.Caption})
		return err

	case msg.Video != nil:
		_, err := b.tb.Send(dst, &telebot.Video{File: msg.Video.File, Caption: msg.Caption})
		return err

	case msg.Document != nil:
		_, err := b.tb.Send(dst, &telebot.Document{
			File:     msg.Document.File,
			Caption:  msg.Caption,
			FileName: msg.Document.FileName,
		})
		return err

	case msg.Sticker != nil:
		_, err := b.tb.Send(dst, &telebot.Sticker{File: msg.Sticker.File})
		return err

	case msg.Audio != nil:
		_, err := b.tb.Send(dst, &telebot.Audio{File: msg.Audio.File, Caption: msg.Caption})
		return err

	case msg.Voice != nil:
		_, err := b.tb.Send(dst, &telebot.Voice{File: msg.Voice.File})
		return err

	case msg.Animation != nil:
		_, err := b.tb.Send(dst, &telebot.Animation{File: msg.Animation.File, Caption: msg.Caption})
		return err

	default:
		b.sendToGroup(msgUnsupportedType)
		return nil
	}
}

// handleUserMessage forwards a user's message to the support group.
func (b *Bot) handleUserMessage(c telebot.Context) error {
	msg := c.Message()

	// Build the user summary card shown above the forwarded message.
	summaryText := b.buildSummaryText(msg.Chat.ID)

	if _, err := b.tb.Send(
		telebot.ChatID(b.cfg.Bot.GroupID),
		summaryText,
		&telebot.SendOptions{ParseMode: telebot.ModeMarkdown},
	); err != nil {
		b.lg.Error("can't send summary to group", zap.Error(err))
	}

	forwardedMsg, err := b.tb.Forward(telebot.ChatID(b.cfg.Bot.GroupID), msg)
	if err != nil {
		b.lg.Error("can't forward message to group", zap.Error(err))
		return c.Send("Не удалось отослать сообщение. Попробуйте ещё раз.")
	}

	if err = b.repo.SaveRequest(forwardedMsg.ID, msg.Chat.ID); err != nil {
		b.lg.Error("can't save support request",
			zap.String("text", msg.Text),
			zap.Error(err),
		)
	}

	return c.Send(msgSentToSupport)
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

// sendToGroup sends a plain text message to the support group, logging errors.
func (b *Bot) sendToGroup(text string) {
	if _, err := b.tb.Send(telebot.ChatID(b.cfg.Bot.GroupID), text); err != nil {
		b.lg.Error("can't send message to group", zap.Error(err))
	}
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
