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
	LastTxID  string
	SubExpire time.Time
}

// Format returns a markdown-formatted card to send to the support group.
func (s *UserSummary) Format() string {
	username := "no"
	if s.Username != "" {
		username = "@" + s.Username
	}

	usedTest := "no"
	if s.UsedTest {
		usedTest = "yes"
	}

	expired := "no"
	if s.Expired {
		expired = "yes"
	}

	sub := "no"
	if s.SubKey != "" {
		sub = fmt.Sprintf("Till %s, `%s`\n%s", s.SubExpire.Format("02.01.2006"), s.SubName, s.SubKey)
	}

	last := ""
	if s.LastTxID != "" {
		last = "💶 Payment: " + s.LastTxID + "\n"
	}

	return fmt.Sprintf(
		"👤 *%s* | `%d`\n"+
			"📅 With us: %s\n"+
			"💰 Balance: *%d₽*\n"+
			"💳 Payments: *%d* on total *%d₽*\n"+
			last+
			"🎁 Test: %s\n"+
			"🐷 Expired: %s\n"+
			"🔐 %s",
		username,
		s.TgID,
		s.JoinedAt.Format("02.01.2006"),
		s.Balance,
		s.PayCount,
		s.PaySum,
		usedTest,
		expired,
		sub,
	)
}
