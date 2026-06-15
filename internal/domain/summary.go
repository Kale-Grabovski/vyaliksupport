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
	Expired   bool
	PayCount  int
	PaySum    int
	SubName   string
	SubKey    string // full subscription URL
	SsSubKey  string // full subscription 1488 URL
	CfSubKey  string // cf subscription URL
	LastTxID  string
	Traffic   int64
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

	expired := "нет"
	if s.Expired {
		expired = "да"
	}

	sub := "нет"
	if s.SubKey != "" {
		sub = fmt.Sprintf("до %s, `%s`\n`%s`", s.SubExpire.Format("02.01.2006"), s.SubName, s.SubKey)
	}

	last := ""
	if s.LastTxID != "" {
		last = "💶 Платеж: " + s.LastTxID + "\n"
	}

	return fmt.Sprintf(
		"👤 *%s* | `%d`\n"+
			"📅 С нами с: %s\n"+
			"💰 Баланс: *%d₽*\n"+
			"🐐 Трафик: %s\n"+
			"💳 Оплат: *%d* на сумму *%d₽*\n"+
			last+
			"🎁 Пробный: %s\n"+
			"🐷 Истекла: %s\n"+
			"🔐 %s\n"+
			"🔐 `%s`\n"+
			"🔐 `%s`",
		username,
		s.TgID,
		s.JoinedAt.Format("02.01.2006"),
		s.Balance,
		formatBytes(s.Traffic),
		s.PayCount,
		s.PaySum,
		usedTest,
		expired,
		sub,
		s.SsSubKey,
		s.CfSubKey,
	)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
