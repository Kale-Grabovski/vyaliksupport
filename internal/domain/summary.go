package domain

import (
	"fmt"
	"time"
)

// UserSummary holds all context about a user needed by the support group.
type UserSummary struct {
	TgID      int64
	Username  string
	JoinedAt  time.Time
	Balance   int
	UsedTest  bool
	PayCount  int
	PaySum    int
	SubName   string
	SubKey    string // full subscription URL
	SubExpire time.Time
}

// Format returns a markdown-formatted card to send to the support group.
func (s *UserSummary) Format() string {
	username := "нет"
	if s.Username != "" {
		username = "@" + s.Username
	}

	usedTest := "нет"
	if s.UsedTest {
		usedTest = "да"
	}

	sub := "нет"
	if s.SubKey != "" {
		sub = fmt.Sprintf("до %s\n`%s`", s.SubExpire.Format("02.01.2006"), s.SubKey)
	}

	return fmt.Sprintf(
		"👤 *%s* | `%d`\n"+
			"📅 С нами с: %s\n"+
			"💰 Баланс: *%d₽*\n"+
			"💳 Оплат: *%d* на сумму *%d₽*\n"+
			"🎁 Пробный: %s\n"+
			"🔐 Подписка: %s",
		username,
		s.TgID,
		s.JoinedAt.Format("02.01.2006"),
		s.Balance,
		s.PayCount,
		s.PaySum,
		usedTest,
		sub,
	)
}
