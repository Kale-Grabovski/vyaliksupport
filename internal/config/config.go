package config

import (
	"time"

	"gopkg.in/telebot.v3"
)

type Config struct {
	Bot Bot `mapstructure:"bot"`
	DB  DB  `mapstructure:"db"`
}

type Bot struct {
	Token        string   `mapstructure:"token"`
	GroupID      int64    `mapstructure:"group_id"`
	NoticeChanID int64    `mapstructure:"notice_channel"`
	Webhook      *Webhook `mapstructure:"webhook"`
}

type Webhook struct {
	URL    string `mapstructure:"url"`
	Listen string `mapstructure:"listen"`
}

func (c Config) BotSettings() telebot.Settings {
	if c.Bot.Webhook != nil {
		return telebot.Settings{
			Token: c.Bot.Token,
			Poller: &telebot.Webhook{
				Listen: "127.0.0.1:" + c.Bot.Webhook.Listen,
				Endpoint: &telebot.WebhookEndpoint{
					PublicURL: c.Bot.Webhook.URL,
				},
			},
		}
	}

	return telebot.Settings{
		Token:  c.Bot.Token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}
}

type DB struct {
	DSN     string `mapstructure:"dsn"`
	Dialect string `mapstructure:"dialect"`
}
