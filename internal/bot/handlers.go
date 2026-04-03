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
	for i := range faqItems {
		idx := i
		btn := &telebot.Btn{Unique: fmt.Sprintf("faq_%d", idx)}
		b.tb.Handle(btn, b.handleFAQAnswer(idx))
	}

	// "Back to FAQ" inline button callback.
	backBtn := &telebot.Btn{Unique: "faq_back"}
	b.tb.Handle(backBtn, b.handleFAQBack)

	// All media types route through the same dispatcher.
	b.tb.Handle(telebot.OnText, b.handleMessage)
	b.tb.Handle(telebot.OnPhoto, b.handleMessage)
	b.tb.Handle(telebot.OnVideo, b.handleMessage)
	b.tb.Handle(telebot.OnDocument, b.handleMessage)
	b.tb.Handle(telebot.OnSticker, b.handleMessage)
	b.tb.Handle(telebot.OnAudio, b.handleMessage)
	b.tb.Handle(telebot.OnVoice, b.handleMessage)
	b.tb.Handle(telebot.OnAnimation, b.handleMessage)
}

// handleStart sends the welcome message with the persistent reply keyboard.
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

// handleFAQAnswer returns a callback handler that answers a specific FAQ item.
func (b *Bot) handleFAQAnswer(idx int) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			b.lg.Warn("can't respond to callback", zap.Error(err))
		}
		return c.Send(faqItems[idx].Answer, &telebot.SendOptions{
			ParseMode:   telebot.ModeMarkdown,
			ReplyMarkup: backKeyboard(),
		})
	}
}

// handleFAQBack handles the inline "back" button.
func (b *Bot) handleFAQBack(c telebot.Context) error {
	if err := c.Respond(); err != nil {
		b.lg.Warn("can't respond to callback", zap.Error(err))
	}
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

// forwardToChatwoot finds or creates a Chatwoot conversation and sends all content.
func (b *Bot) forwardToChatwoot(c telebot.Context) error {
	if b.woot == nil {
		return nil
	}

	user := c.Sender()
	msg := c.Message()

	identifier := fmt.Sprintf("tg:%d", user.ID)
	inboxID := b.cfg.Chatwoot.InboxID
	accountID := b.cfg.Chatwoot.AccountID

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

	// Build prefix with user info
	var prefix string
	if user.Username != "" {
		prefix = fmt.Sprintf("@%s (%s):", user.Username, user.FirstName)
	} else {
		prefix = fmt.Sprintf("%s (ID: %d):", user.FirstName, user.ID)
	}

	// Collect attachments from media
	var attachments []chatwoot.AttachmentInfo
	var text string
	var msgType string

	switch {
	case msg.Photo != nil:
		text = msg.Caption
		msgType = "photo"
		att := b.getAttachmentFromFile(msg.Photo.File.FileID, msg.Photo.File.FileID+".jpg")
		if att.URL != "" {
			att.MimeType = "image/jpeg"
			attachments = append(attachments, att)
		}

	case msg.Video != nil:
		text = msg.Caption
		msgType = "video"
		att := b.getAttachmentFromFile(msg.Video.File.FileID, msg.Video.FileName)
		if att.URL != "" {
			attachments = append(attachments, att)
		}

	case msg.Document != nil:
		text = msg.Caption
		msgType = "document"
		att := b.getAttachmentFromFile(msg.Document.FileID, msg.Document.FileName)
		if att.URL != "" {
			attachments = append(attachments, att)
		}

	case msg.Sticker != nil:
		text = ""
		msgType = "sticker"
		// Get the actual sticker file (will be .webp or .tgs)
		att := b.getAttachmentFromFile(msg.Sticker.FileID, msg.Sticker.UniqueID+".webp")
		if att.URL != "" {
			att.MimeType = "image/webp"
			attachments = append(attachments, att)
		}

	case msg.Audio != nil:
		text = msg.Caption
		msgType = "audio"
		att := b.getAttachmentFromFile(msg.Audio.File.FileID, msg.Audio.FileName)
		if att.URL != "" {
			attachments = append(attachments, att)
		}

	case msg.Voice != nil:
		text = ""
		msgType = "voice"
		att := b.getAttachmentFromFile(msg.Voice.FileID, msg.Voice.FileID+".ogg")
		if att.URL != "" {
			att.MimeType = "audio/ogg"
			attachments = append(attachments, att)
		}

	case msg.Animation != nil:
		text = msg.Caption
		msgType = "animation"
		att := b.getAttachmentFromFile(msg.Animation.File.FileID, msg.Animation.FileName)
		if att.URL != "" {
			attachments = append(attachments, att)
		}

	default:
		text = msg.Text
		msgType = "text"
	}

	// Build content
	content := prefix
	if text != "" {
		content += "\n" + text
	}

	// Send with or without attachments - use single multipart request
	var sendErr error
	if len(attachments) > 0 {
		// Send everything in ONE request: text + all attachments together
		sendErr = b.woot.SendMessageWithAttachments(accountID, convID, content, attachments)
	} else {
		if content == prefix {
			content = prefix + "\n[empty message]"
		}
		sendErr = b.woot.SendMessage(accountID, convID, content)
	}

	if sendErr != nil {
		b.lg.Error("failed to send message to Chatwoot",
			zap.Int64("user_id", user.ID),
			zap.Int("conv_id", convID),
			zap.Error(sendErr),
		)
		return c.Send(msgSentToSupport, &telebot.SendOptions{
			ReplyMarkup: mainKeyboard(),
		})
	}

	b.lg.Info("forwarded message to Chatwoot",
		zap.Int64("user_id", user.ID),
		zap.Int("conv_id", convID),
		zap.String("type", msgType),
		zap.Int("attachments", len(attachments)),
	)

	return c.Send(msgSentToSupport, &telebot.SendOptions{
		ReplyMarkup: mainKeyboard(),
	})
}

// getMsgType returns message type string.
func getMsgType(msg *telebot.Message) string {
	switch {
	case msg.Photo != nil:
		return "photo"
	case msg.Video != nil:
		return "video"
	case msg.Document != nil:
		return "document"
	case msg.Sticker != nil:
		return "sticker"
	case msg.Audio != nil:
		return "audio"
	case msg.Voice != nil:
		return "voice"
	case msg.Animation != nil:
		return "animation"
	default:
		return "text"
	}
}

// getAttachmentFromFile gets file URL from bot and creates attachment.
func (b *Bot) getAttachmentFromFile(fileID, filename string) chatwoot.AttachmentInfo {
	file, err := b.tb.FileByID(fileID)
	if err != nil {
		b.lg.Warn("failed to get file URL", zap.String("file_id", fileID), zap.Error(err))
		return chatwoot.AttachmentInfo{}
	}
	return chatwoot.AttachmentInfo{
		URL:      file.FileURL,
		FileName: filename,
	}
}

// faqKeyboard builds the inline keyboard for the FAQ menu.
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
func mainKeyboard() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{ResizeKeyboard: true}
	home := markup.Text(btnLabelHome)
	faq := markup.Text(btnLabelFAQ)
	markup.Reply(markup.Row(home, faq))
	return markup
}
