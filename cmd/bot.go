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

	"vyaliksupport/internal/config"
	"vyaliksupport/pkg/db/postgres"
)

var botCmd = &cobra.Command{
	Use: "support",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := loadConfig(cfgFile)
		if err != nil {
			panic(err)
		}

		lgCfg := zap.NewProductionConfig()
		lgCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
		lg, _ := lgCfg.Build()
		defer lg.Sync()

		db, err := connectDB(cfg.DB.DSN, cfg.DB.Dialect)
		if err != nil {
			lg.Error("can't connect to DB: "+cfg.DB.DSN, zap.Error(err))
			return
		}
		defer db.Close()

		tb, err := telebot.NewBot(cfg.BotSettings())
		if err != nil {
			lg.Error("can't init TG bot", zap.Error(err))
			return
		}

		reqRepo := postgres.NewReq(db)
		err = reqRepo.Migrate()
		if err != nil {
			lg.Error("can't migrate", zap.Error(err))
			return
		}

		handleMessage := func(c telebot.Context) error {
			msg := c.Message()

			// Reply on message => sending to user
			if msg.Chat.ID == cfg.Bot.GroupID && msg.ReplyTo != nil {
				repliedMsgID := msg.ReplyTo.ID

				userChatID, err := reqRepo.FindUserChatID(repliedMsgID)
				if err != nil {
					return c.Send("❌ Не могу найти ваш запрос")
				}

				// 1st message
				_, err = tb.Send(telebot.ChatID(userChatID), "👨‍💻 Ответ из поддержки:")
				if err != nil {
					lg.Error("can't send response to user", zap.Int64("userChatID", userChatID), zap.Error(err))
					return c.Send("❌ Не удалось отправить ответ пользователю. Возможно, он заблокировал бота.")
				}

				// 2nd message
				switch {
				case msg.Text != "":
					_, err = tb.Send(telebot.ChatID(userChatID), msg.Text, &telebot.SendOptions{
						ParseMode: telebot.ModeMarkdown,
					})

				case msg.Photo != nil:
					_, err = tb.Send(telebot.ChatID(userChatID), &telebot.Photo{
						File:    msg.Photo.File,
						Caption: msg.Caption,
					})
				case msg.Video != nil:
					_, err = tb.Send(telebot.ChatID(userChatID), &telebot.Video{
						File:    msg.Video.File,
						Caption: msg.Caption,
					})
				case msg.Document != nil:
					_, err = tb.Send(telebot.ChatID(userChatID), &telebot.Document{
						File:     msg.Document.File,
						Caption:  msg.Caption,
						FileName: msg.Document.FileName,
					})
				case msg.Sticker != nil:
					_, err = tb.Send(telebot.ChatID(userChatID), &telebot.Sticker{
						File: msg.Sticker.File,
					})
				case msg.Audio != nil:
					_, err = tb.Send(telebot.ChatID(userChatID), &telebot.Audio{
						File:    msg.Audio.File,
						Caption: msg.Caption,
					})
				default:
					_, err = tb.Send(telebot.ChatID(userChatID), "📎 [Неподдерживаемый тип сообщения]")
				}

				if err != nil {
					lg.Error("can't send response to user", zap.Int64("userChatID", userChatID), zap.Error(err))
					return c.Send("❌ Не удалось отправить ответ пользователю. Возможно, он заблокировал бота.")
				}

				return c.Send("✅ Ответ отправлен пользователю.")
			}

			if msg.Text == "/start" {
				return c.Send("Отправьте ваш вопрос одним сообщением")
			}

			// Message to group
			if msg.Chat.ID != cfg.Bot.GroupID {
				msgPrev := fmt.Sprintf("💬 Новое сообщение от пользователя #%d", msg.Chat.ID)
				tb.Send(telebot.ChatID(cfg.Bot.GroupID), msgPrev)

				forwardedMsg, err := tb.Forward(telebot.ChatID(cfg.Bot.GroupID), msg)
				if err != nil {
					lg.Error("can't forward message", zap.Error(err))
					return c.Send("Не удалось отослать сообщение. Попробуйте ещё раз.")
				}

				err = reqRepo.SaveRequest(forwardedMsg.ID, msg.Chat.ID)
				if err != nil {
					lg.Error("can't save message", zap.String("msg", msg.Text), zap.Error(err))
				}

				//go func() {
				//	notice := fmt.Sprintf("New message from user %d/%s\n%s", msg.Chat.ID, msg.Chat.Username, msg)
				//	_, err = tb.Send(telebot.ChatID(cfg.Bot.NoticeChanID), notice)
				//	if err != nil {
				//		lg.Error("cannot send notice: %v", zap.Error(err))
				//	}
				//}()
				return c.Send("✅ Сообщение поддержке отправлено")
			}

			return nil
		}

		tb.Handle(telebot.OnText, handleMessage)
		tb.Handle(telebot.OnPhoto, handleMessage)
		tb.Handle(telebot.OnVideo, handleMessage)
		tb.Handle(telebot.OnDocument, handleMessage)
		tb.Handle(telebot.OnSticker, handleMessage)
		tb.Handle(telebot.OnAudio, handleMessage)
		tb.Handle(telebot.OnVoice, handleMessage)
		tb.Handle(telebot.OnAnimation, handleMessage)

		go tb.Start()

		lg.Info("Bot started")

		c := make(chan os.Signal, 2)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c

		tb.Stop()
		lg.Info("Bot finished")
	},
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
