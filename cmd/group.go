package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vyaliksupport/internal/bot"
	"vyaliksupport/internal/listener"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/telebot.v4"
)

var groupCmd = &cobra.Command{
	Use:  "group",
	RunE: runGroup,
}

func runGroup(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	lgCfg := zap.NewProductionConfig()
	lgCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	lg, err := lgCfg.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}
	defer lg.Sync()

	db, err := connectDB(cfg.DB.DSN, cfg.DB.Dialect)
	if err != nil {
		lg.Error("can't connect to DB", zap.String("dsn", cfg.DB.DSN), zap.Error(err))
		return err
	}
	defer db.Close()

	tb, err := telebot.NewBot(cfg.BotSettings())
	if err != nil {
		lg.Error("can't init TG bot", zap.Error(err))
		return err
	}

	// Initialize Ntfy sender (to user via bot) and listener (from user via bot).
	ntfySender := sender.NewNtfySender(cfg.Ntfy.TopicUserToGroup, cfg.Ntfy.Token, cfg.Ntfy.EncryptKey)
	ntfyListener := listener.NewNtfyListener(cfg.Ntfy.TopicGroupToUser, cfg.Ntfy.Token, cfg.Ntfy.EncryptKey, lg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ntfyListener.Start(ctx); err != nil {
		lg.Error("can't start ntfy listener", zap.Error(err))
		return err
	}
	defer ntfyListener.Stop()

	repo := postgres.NewReq(db, cfg.Bot.SubHost)
	g := groupHandler{
		tb:           tb,
		groupID:      cfg.Bot.GroupID,
		repo:         repo,
		ntfySender:   ntfySender,
		ntfyListener: ntfyListener,
		lg:           lg,
	}

	// Handle incoming messages from ntfy (user messages to forward to group).
	go g.handleIncomingMessages(ctx)

	// Register group reply handler.
	g.registerHandlers()

	lg.Info("Group handler started")

	// Start cleanup job.
	go g.runCleanup(ctx, repo, lg)

	// Start bot poller.
	go tb.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	cancel()
	tb.Stop()
	lg.Info("Group handler stopped")
	return nil
}

// groupHandler handles messages for the group service.
type groupHandler struct {
	tb           *telebot.Bot
	groupID      int64
	repo         *postgres.Req
	ntfySender   *sender.NtfySender
	ntfyListener *listener.NtfyListener
	lg           *zap.Logger
}

// handleIncomingMessages processes messages received from ntfy (from bot service).
func (g *groupHandler) handleIncomingMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-g.ntfyListener.Messages():
			if payload == nil {
				continue
			}
			g.forwardToGroup(payload)
		}
	}
}

// forwardToGroup sends the payload content to the TG group.
func (g *groupHandler) forwardToGroup(payload *bot.Payload) {
	var groupMsgID int

	// Send summary first.
	if payload.Summary != "" {
		msg, err := g.tb.Send(telebot.ChatID(g.groupID), payload.Summary, &telebot.SendOptions{
			ParseMode: telebot.ModeMarkdown,
		})
		if err != nil {
			g.lg.Error("can't send summary to group", zap.Error(err))
		} else {
			groupMsgID = msg.ID
		}
	}

	dst := telebot.ChatID(g.groupID)

	// Send the content based on type.
	switch payload.Content.Type {
	case bot.ContentTypeText:
		if payload.Content.Text != "" {
			msg, err := g.tb.Send(dst, payload.Content.Text, &telebot.SendOptions{
				ParseMode: telebot.ModeMarkdown,
			})
			if err != nil {
				g.lg.Error("can't send text to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypePhoto:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Photo{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				g.lg.Error("can't send photo to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeVideo:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Video{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				g.lg.Error("can't send video to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeDocument:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Document{
				File:     telebot.File{FileID: payload.Content.FileID},
				Caption:  payload.Content.Caption,
				FileName: payload.Content.FileName,
			})
			if err != nil {
				g.lg.Error("can't send document to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeSticker:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Sticker{File: telebot.File{FileID: payload.Content.FileID}})
			if err != nil {
				g.lg.Error("can't send sticker to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeAudio:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Audio{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				g.lg.Error("can't send audio to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeVoice:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Voice{File: telebot.File{FileID: payload.Content.FileID}})
			if err != nil {
				g.lg.Error("can't send voice to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}

	case bot.ContentTypeAnimation:
		if payload.Content.FileID != "" {
			msg, err := g.tb.Send(dst, &telebot.Animation{File: telebot.File{FileID: payload.Content.FileID}, Caption: payload.Content.Caption})
			if err != nil {
				g.lg.Error("can't send animation to group", zap.Error(err))
			} else {
				groupMsgID = msg.ID
			}
		}
	}

	// Save mapping: group_message_id -> user_chat_id locally (group's own DB).
	if groupMsgID > 0 {
		if err := g.repo.SaveGroupMessage(groupMsgID, payload.UserChatID); err != nil {
			g.lg.Error("can't save group message mapping", zap.Error(err))
		}
	}
}

// registerHandlers sets up handlers for group replies.
func (g *groupHandler) registerHandlers() {
	// Handle replies in the group.
	g.tb.Handle(telebot.OnText, g.handleGroupMessage)
	g.tb.Handle(telebot.OnPhoto, g.handleGroupMedia)
	g.tb.Handle(telebot.OnVideo, g.handleGroupMedia)
	g.tb.Handle(telebot.OnDocument, g.handleGroupMedia)
	g.tb.Handle(telebot.OnSticker, g.handleGroupMedia)
	g.tb.Handle(telebot.OnAudio, g.handleGroupMedia)
	g.tb.Handle(telebot.OnVoice, g.handleGroupMedia)
	g.tb.Handle(telebot.OnAnimation, g.handleGroupMedia)
}

// handleGroupMessage handles text messages in the group.
func (g *groupHandler) handleGroupMessage(c telebot.Context) error {
	msg := c.Message()

	// Only handle messages in the group.
	if msg.Chat.ID != g.groupID {
		return nil
	}

	// Only handle replies.
	if msg.ReplyTo == nil {
		return nil
	}

	return g.handleGroupReply(c, bot.ContentTypeText, msg.Text, "", "", "")
}

// handleGroupMedia handles media messages in the group.
func (g *groupHandler) handleGroupMedia(c telebot.Context) error {
	msg := c.Message()

	// Only handle messages in the group.
	if msg.Chat.ID != g.groupID {
		return nil
	}

	// Only handle replies.
	if msg.ReplyTo == nil {
		return nil
	}

	contentType := bot.ContentTypeText
	var fileID, caption, fileName string

	if msg.Photo != nil {
		contentType = bot.ContentTypePhoto
		fileID = msg.Photo.File.FileID
		caption = msg.Caption
	} else if msg.Video != nil {
		contentType = bot.ContentTypeVideo
		fileID = msg.Video.File.FileID
		caption = msg.Caption
	} else if msg.Document != nil {
		contentType = bot.ContentTypeDocument
		fileID = msg.Document.File.FileID
		caption = msg.Caption
		fileName = msg.Document.FileName
	} else if msg.Sticker != nil {
		contentType = bot.ContentTypeSticker
		fileID = msg.Sticker.File.FileID
	} else if msg.Audio != nil {
		contentType = bot.ContentTypeAudio
		fileID = msg.Audio.File.FileID
		caption = msg.Caption
	} else if msg.Voice != nil {
		contentType = bot.ContentTypeVoice
		fileID = msg.Voice.File.FileID
	} else if msg.Animation != nil {
		contentType = bot.ContentTypeAnimation
		fileID = msg.Animation.File.FileID
		caption = msg.Caption
	}

	return g.handleGroupReply(c, contentType, "", fileID, caption, fileName)
}

// handleGroupReply sends a reply from the group back to the user via ntfy.
func (g *groupHandler) handleGroupReply(c telebot.Context, contentType, text, fileID, caption, fileName string) error {
	msg := c.Message()

	// Find user_chat_id by the group message we're replying to.
	userChatID, err := g.repo.FindUserChatIDByGroupMsg(msg.ReplyTo.ID)
	if err != nil {
		if _, err := g.tb.Send(telebot.ChatID(g.groupID), "❌ can't find user"); err != nil {
			g.lg.Error("can't send error to group", zap.Error(err))
		}
		return nil
	}

	// Create payload to send back to bot.
	payload := &bot.Payload{
		Direction:  bot.DirectionToUser,
		UserChatID: userChatID,
		MsgID:      msg.ReplyTo.ID, // This helps bot find which message to reply to.
		Content: bot.Content{
			Type:     contentType,
			Text:     text,
			FileID:   fileID,
			Caption:  caption,
			FileName: fileName,
		},
		CreatedAt: time.Now(),
	}

	// Send to ntfy (topic-in).
	data, err := payload.Marshal()
	if err != nil {
		g.lg.Error("can't marshal payload", zap.Error(err))
		return err
	}

	if err := g.ntfySender.SendPayload(context.Background(), data); err != nil {
		g.lg.Error("can't send payload to ntfy", zap.Error(err))
		return err
	}

	if _, err := g.tb.Send(telebot.ChatID(g.groupID), "✅ message sent to user"); err != nil {
		g.lg.Error("can't send confirmation to group", zap.Error(err))
	}

	return nil
}

// runCleanup periodically removes expired requests.
func (g *groupHandler) runCleanup(ctx context.Context, repo *postgres.Req, lg *zap.Logger) {
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

func init() {
	rootCmd.AddCommand(groupCmd)
}
