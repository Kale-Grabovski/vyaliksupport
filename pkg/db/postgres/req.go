package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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
	query := "INSERT INTO tg_support_requests (support_message_id, user_chat_id) VALUES ($1, $2)"
	_, err := d.db.Exec(query, supportMessageID, userChatID)
	return err
}

func (d *Req) FindUserChatID(supportMessageID int) (userChatID int64, err error) {
	query := "SELECT user_chat_id FROM tg_support_requests WHERE support_message_id = $1"
	err = d.db.QueryRow(query, supportMessageID).Scan(&userChatID)
	return
}

// IsUserBanned checks if a user is in the ban list.
func (r *Req) IsUserBanned(tgID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	q := `SELECT EXISTS(SELECT 1 FROM tg_support_ban WHERE tg_id = $1)`
	err := r.db.QueryRowContext(ctx, q, tgID).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

// BanUser adds a user to the ban list.
func (r *Req) BanUser(tgID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := `INSERT INTO tg_support_ban (tg_id) VALUES ($1) ON CONFLICT (tg_id) DO NOTHING`
	_, err := r.db.ExecContext(ctx, q, tgID)
	return err
}

// UnbanUser removes a user from the ban list.
func (r *Req) UnbanUser(tgID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := `DELETE FROM tg_support_ban WHERE tg_id = $1`
	_, err := r.db.ExecContext(ctx, q, tgID)
	return err
}

// GetUserSummary collects all info about the user from the main DB.
func (r *Req) GetUserSummary(tgID int64) (*domain.UserSummary, error) {
	s := &domain.UserSummary{}

	// Base user info + payment stats in one query.
	row := r.db.QueryRow(`
		SELECT
			u.tg_id,
			coalesce(u.username, ''),
			u.created_at,
			u.balance,
			u.used_test,
			(
				SELECT count(id)
				FROM tg_payments
				WHERE tg_id = u.tg_id AND status = 'ok' AND paid_amount > 0
			) AS pay_count,
			(
				SELECT coalesce(sum(paid_amount), 0)
				FROM tg_payments
				WHERE tg_id = u.tg_id AND status = 'ok' AND paid_amount > 0
			) AS pay_sum,
			coalesce((
				SELECT tx_id
				FROM tg_payments
				WHERE tg_id = u.tg_id AND status = 'ok'
				ORDER BY created_at DESC
				LIMIT 1
			), '') AS last_tx_id,
			(
				SELECT coalesce(sum(used_traffic_bytes), 0)
				FROM user_traffic
				WHERE t_id IN (SELECT t_id FROM users WHERE telegram_id = u.tg_id)
			) AS used_traffic_bytes
		FROM tg_users AS u
		WHERE u.tg_id = $1
	`, tgID)

	err := row.Scan(&s.TgID, &s.Username, &s.JoinedAt, &s.Balance, &s.UsedTest,
		&s.PayCount, &s.PaySum, &s.LastTxID, &s.Traffic)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	var shortUUID sql.NullString
	var subName sql.NullString
	var expireAt sql.NullTime

	err = r.db.QueryRow(`
		SELECT short_uuid, username, expire_at, expire_at < now()
		FROM users
		WHERE telegram_id = $1
		ORDER BY expire_at DESC
		LIMIT 1
	`, tgID).Scan(&shortUUID, &subName, &expireAt, &s.Expired)

	if err == nil && shortUUID.Valid {
		s.SubName = subName.String
		s.SubExpire = expireAt.Time
		s.SubKey = fmt.Sprintf("%s/%s", r.subHost, shortUUID.String)

		s.SsSubKey = fmt.Sprintf("%s:1488/ss/%s", r.subHost, shortUUID.String)
		s.SsSubKey = strings.Replace(s.SsSubKey, "sub.", "", -1)

		cf := strings.Replace(strings.Replace(r.subHost, "sub.", "", -1), "https://", "", -1)
		s.CfSubKey = fmt.Sprintf("https://sbb.%s/ss/%s", cf, shortUUID.String)
	}

	return s, nil
}

func (d *Req) Migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS tg_support_requests (
			id SERIAL PRIMARY KEY,
			support_message_id INTEGER NOT NULL,
			user_chat_id BIGINT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_support_message_id ON tg_support_requests(support_message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_chat_id ON tg_support_requests(user_chat_id)`,

		`CREATE TABLE IF NOT EXISTS tg_support_ban (
			id serial PRIMARY KEY,
			tg_id bigint NOT NULL UNIQUE,
			created_at timestamptz(0) DEFAULT now()
		)`,
	}
	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
