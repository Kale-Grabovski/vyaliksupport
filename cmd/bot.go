package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/telebot.v4"

	"vyaliksupport/internal/bot"
	"vyaliksupport/internal/config"
	"vyaliksupport/pkg/db/postgres"
)

var botCmd = &cobra.Command{
	Use:  "support",
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

	b := bot.New(tb, cfg, repo, lg)

	go b.Start()
	lg.Info("Bot started")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

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
