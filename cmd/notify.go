package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"vyaliksupport/internal/listener"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/telebot.v4"
)

var notifyCmd = &cobra.Command{
	Use:  "notify",
	RunE: runNotify,
}

func runNotify(cmd *cobra.Command, args []string) error {
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

	lg.Info("Starting notify service", zap.Int64("channel_id", cfg.Bot.ChannelID), zap.String("topic", cfg.Ntfy.TopicNotifications))

	tb, err := telebot.NewBot(cfg.BotSettings())
	if err != nil {
		lg.Error("can't init TG bot", zap.Error(err))
		return err
	}

	// Initialize Ntfy listener for notifications topic.
	ntfyListener := listener.NewNtfyListener(cfg.Ntfy.TopicNotifications, cfg.Ntfy.Token, cfg.Ntfy.EncryptKey, lg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ntfyListener.Start(ctx); err != nil {
		lg.Error("can't start ntfy listener", zap.Error(err))
		return err
	}
	defer ntfyListener.Stop()

	n := notifyHandler{
		tb:           tb,
		channelID:    cfg.Bot.ChannelID,
		ntfyListener: ntfyListener,
		lg:           lg,
	}

	// Handle incoming messages from ntfy.
	go n.handleIncomingMessages(ctx)

	lg.Info("Notify handler started")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	cancel()
	lg.Info("Notify handler stopped")
	return nil
}

// notifyHandler handles messages for the notify service.
type notifyHandler struct {
	tb           *telebot.Bot
	channelID    int64
	ntfyListener *listener.NtfyListener
	lg           *zap.Logger
}

// handleIncomingMessages processes messages received from ntfy and forwards to channel.
func (n *notifyHandler) handleIncomingMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-n.ntfyListener.Messages():
			if payload == nil {
				continue
			}
			n.forwardToChannel(payload)
		}
	}
}

// forwardToChannel sends the payload content to the TG channel as plain text.
func (n *notifyHandler) forwardToChannel(payload *Payload) {
	// Send the text content as-is.
	text := payload.Content.Text
	if text == "" && payload.Summary != "" {
		text = payload.Summary
	}

	if text == "" {
		n.lg.Warn("no text content to send to channel")
		return
	}

	dst := telebot.ChatID(n.channelID)
	if _, err := n.tb.Send(dst, text); err != nil {
		n.lg.Error("can't send text to channel", zap.Error(err))
	}
}

func init() {
	rootCmd.AddCommand(notifyCmd)
}
