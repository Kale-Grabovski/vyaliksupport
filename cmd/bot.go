package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"vyaliksupport/internal/bot"
	"vyaliksupport/internal/config"
	"vyaliksupport/internal/domain"
	"vyaliksupport/internal/listener"
	"vyaliksupport/internal/sender"
	"vyaliksupport/pkg/db/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/telebot.v4"
)

var botCmd = &cobra.Command{
	Use:  "bot",
	RunE: runBot,
}

func runBot(cmd *cobra.Command, args []string) error {
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
	defer lg.Sync() //nolint:errcheck

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

	repo := postgres.NewReq(db, cfg.Bot.SubHost)
	if err = repo.Migrate(); err != nil {
		lg.Error("can't migrate", zap.Error(err))
		return err
	}

	// Initialize Ntfy sender (for topic-out) and listener (for topic-in).
	ntfySender := sender.NewNtfySender(cfg.Ntfy.TopicOut, cfg.Ntfy.Token, cfg.Ntfy.EncryptKey)
	ntfyListener := listener.NewNtfyListener(cfg.Ntfy.TopicIn, cfg.Ntfy.Token, cfg.Ntfy.EncryptKey, lg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ntfyListener.Start(ctx); err != nil {
		lg.Error("can't start ntfy listener", zap.Error(err))
		return err
	}
	defer ntfyListener.Stop()

	b := bot.New(tb, cfg, repo, ntfySender, lg)

	// Handle incoming messages from ntfy (replies from group).
	go b.HandleIncomingMessages(ctx, ntfyListener)

	go b.Start()
	lg.Info("Bot started")

	// Start cleanup job.
	go b.RunCleanup(ctx, repo, lg)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	cancel()
	b.Stop()
	lg.Info("Bot stopped")
	return nil
}

func init() {
	rootCmd.AddCommand(botCmd)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "app.yaml", "config yaml file")
}

func loadConfig(path string) (cfg config.Config, err error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	err = viper.ReadInConfig()
	if err != nil {
		return cfg, fmt.Errorf("cannot read config: %w", err)
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	err = viper.Unmarshal(&cfg)
	if err != nil {
		return cfg, fmt.Errorf("cannot unmarshal config: %w", err)
	}
	return
}

func connectDB(dsn, dialect string) (*sqlx.DB, error) {
	db, err := sqlx.Connect(dialect, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

// Payload and Content are re-exported for convenience.
type Payload = domain.Payload
type Content = domain.Content
