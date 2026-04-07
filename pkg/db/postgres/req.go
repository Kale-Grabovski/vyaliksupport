package postgres

import (
	"database/sql"
	"fmt"

	"vyaliksupport/internal/domain"

	"github.com/jmoiron/sqlx"
)

type Req struct {
	db      *sqlx.DB
	subHost string // e.g. "https://sub.example.com"
}

func NewReq(db *sqlx.DB, subHost string) *Req {
	return &Req{db: db, subHost: subHost}
}

func (d *Req) SaveRequest(supportMessageID int, userChatID int64) error {
	return d.SaveRequestWithTTL(supportMessageID, userChatID)
}

// SaveGroupMessage stores the mapping: group_message_id -> user_chat_id.
// This is needed to find the user when support replies in the group.
func (d *Req) SaveGroupMessage(groupMessageID int, userChatID int64) error {
	query := "INSERT INTO tg_support_requests (group_message_id, user_chat_id) VALUES ($1, $2)"
	_, err := d.db.Exec(query, groupMessageID, userChatID)
	return err
}

// FindUserChatIDByGroupMsg finds user_chat_id by the message in the group.
func (d *Req) FindUserChatIDByGroupMsg(groupMessageID int) (userChatID int64, err error) {
	query := "SELECT user_chat_id FROM tg_support_requests WHERE group_message_id = $1"
	err = d.db.QueryRow(query, groupMessageID).Scan(&userChatID)
	return
}

// FindUserChatID finds user_chat_id by the original user message ID (legacy).
func (d *Req) FindUserChatID(supportMessageID int) (userChatID int64, err error) {
	query := "SELECT user_chat_id FROM tg_support_requests WHERE support_message_id = $1"
	err = d.db.QueryRow(query, supportMessageID).Scan(&userChatID)
	return
}

// GetUserSummary collects all info about the user from the main DB.
func (d *Req) GetUserSummary(tgID int64) (*domain.UserSummary, error) {
	s := &domain.UserSummary{TgID: tgID}

	// Base user info + payment stats in one query.
	row := d.db.QueryRow(`
		SELECT
			coalesce(u.username, ''),
			u.created_at,
			u.balance,
			u.used_test,
			count(p.id) AS pay_count,
			coalesce(sum(p.paid_amount), 0) AS pay_sum,
			coalesce((
				SELECT tx_id
				FROM tg_payments
				WHERE tg_id = $1 AND status = 'ok' AND payment_system = 'platega'
				ORDER BY created_at DESC
				LIMIT 1
			), '') AS last_tx_id
		FROM tg_users AS u
		LEFT JOIN tg_payments AS p
			ON p.tg_id = u.tg_id AND p.status = 'ok' AND p.paid_amount > 0
		WHERE u.tg_id = $1
		GROUP BY u.username, u.created_at, u.balance, u.used_test
	`, tgID)

	err := row.Scan(&s.Username, &s.JoinedAt, &s.Balance, &s.UsedTest, &s.PayCount, &s.PaySum, &s.LastTxID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Active subscription (most recent non-expired, unlimited traffic only).
	var shortUUID sql.NullString
	var subName sql.NullString
	var expireAt sql.NullTime

	err = d.db.QueryRow(`
		SELECT short_uuid, username, expire_at, expire_at < now()
		FROM users
		WHERE telegram_id = $1
		ORDER BY expire_at DESC
		LIMIT 1
	`, tgID).Scan(&shortUUID, &subName, &expireAt, &s.Expired)

	if err == nil && shortUUID.Valid {
		s.SubName = subName.String
		s.SubExpire = expireAt.Time
		s.SubKey = fmt.Sprintf("%s/%s", d.subHost, shortUUID.String)
	}

	return s, nil
}

func (d *Req) Migrate() error {
	queries := []string{
		`DROP TABLE IF EXISTS tg_support_requests`,
		`CREATE TABLE IF NOT EXISTS tg_support_requests (
			id bigserial PRIMARY KEY,
			support_message_id bigint NOT NULL DEFAULT 0,
			group_message_id bigint NOT NULL DEFAULT 0,
			user_chat_id bigint NOT NULL,
			created_at timestamptz DEFAULT now(),
			expires_at timestamptz NOT NULL DEFAULT now() + INTERVAL '72 hours'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_support_message_id ON tg_support_requests(support_message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_group_message_id ON tg_support_requests(group_message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_chat_id ON tg_support_requests(user_chat_id)`,
		`CREATE INDEX IF NOT EXISTS idx_expires_at ON tg_support_requests(expires_at)`,
	}
	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Cleanup removes requests that have expired.
func (d *Req) Cleanup() (int64, error) {
	result, err := d.db.Exec("DELETE FROM tg_support_requests WHERE expires_at < NOW()")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SaveRequestWithTTL creates a new request with a specific TTL.
func (d *Req) SaveRequestWithTTL(supportMessageID int, userChatID int64) error {
	query := "INSERT INTO tg_support_requests (support_message_id, user_chat_id) VALUES ($1, $2)"
	_, err := d.db.Exec(query, supportMessageID, userChatID)
	return err
}
