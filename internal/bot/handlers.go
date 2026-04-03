package bot

import (
	"fmt"

	"go.uber.org/zap"
	"gopkg.in/telebot.v4"

	"vyaliksupport/internal/chatwoot"
	"vyaliksupport/internal/config"
	"vyaliksupport/pkg/db/postgres"
)

// Bot holds all dependencies needed by the handlers.
type Bot struct {
	tb   *telebot.Bot
	cfg  config.Config
	repo *postgres.Req
	lg   *zap.Logger
	woot *chatwoot.Woot
}

// New creates a Bot and registers all handlers.
func New(tb *telebot.Bot, cfg config.Config, repo *postgres.Req, lg *zap.Logger, woot *chatwoot.Woot) *Bot {
	b := &Bot{
		tb:   tb,
		cfg:  cfg,
		repo: repo,
		lg:   lg,
		woot: woot,
	}
	b.registerHandlers()
	return b
}

// TelegramBot returns the underlying *telebot.Bot for use by the Chatwoot webhook handler.
func (b *Bot) TelegramBot() *telebot.Bot {
	return b.tb
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

// handleMessage handles all user messages — FAQ/start commands and Chatwoot forwarding.
func (b *Bot) handleMessage(c telebot.Context) error {
	msg := c.Message()

	// Handle bot commands and keyboard buttons locally.
	if msg.Text != "" {
		switch msg.Text {
		case "/start", btnLabelHome:
			return b.handleStart(c)
		case "/faq", btnLabelFAQ:
			return b.handleFAQ(c)
		}
	}

	// Forward the user's message to Chatwoot.
	return b.forwardToChatwoot(c)
}

// forwardToChatwoot finds or creates a Chatwoot conversation for this Telegram user
// and sends the message content to it.
func (b *Bot) forwardToChatwoot(c telebot.Context) error {
	if b.woot == nil {
		return nil
	}

	user := c.Sender()
	msg := c.Message()

	// Build the user identifier — Chatwoot uses this to find/create the conversation.
	identifier := fmt.Sprintf("tg:%d", user.ID)

	// Get the Chatwoot inbox for this integration.
	inboxID := b.cfg.Chatwoot.InboxID
	accountID := b.cfg.Chatwoot.AccountID

	// Find or create the conversation.
	convID, err := b.woot.FindOrCreateConversation(accountID, inboxID, identifier)
	if err != nil {
		b.lg.Error("failed to find or create Chatwoot conversation",
			zap.Int64("user_id", user.ID),
			zap.String("identifier", identifier),
			zap.Error(err),
		)
		return c.Send(msgSentToSupport, &telebot.SendOptions{
			ReplyMarkup: mainKeyboard(),
		})
	}

	// Build the message content. Include the user's display name for context.
	var content string
	if user.Username != "" {
		content = fmt.Sprintf("@%s (%s):\n%s", user.Username, user.FirstName, msg.Text)
	} else {
		content = fmt.Sprintf("%s (ID: %d):\n%s", user.FirstName, user.ID, msg.Text)
	}
	if content == "" {
		content = "[empty message]"
	}

	// Send the message to Chatwoot.
	if err := b.woot.SendMessage(accountID, convID, content); err != nil {
		b.lg.Error("failed to send message to Chatwoot",
			zap.Int64("user_id", user.ID),
			zap.Int("conv_id", convID),
			zap.Error(err),
		)
		return c.Send(msgSentToSupport, &telebot.SendOptions{
			ReplyMarkup: mainKeyboard(),
		})
	}

	b.lg.Info("forwarded message to Chatwoot",
		zap.Int64("user_id", user.ID),
		zap.Int("conv_id", convID),
	)

	return c.Send(msgSentToSupport, &telebot.SendOptions{
		ReplyMarkup: mainKeyboard(),
	})
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
