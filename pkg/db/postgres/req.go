package postgres

import (
	"github.com/jmoiron/sqlx"
)

type Req struct {
	db *sqlx.DB
}

func NewReq(db *sqlx.DB) *Req {
	return &Req{db: db}
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
	}
	for _, q := range queries {
		_, err := d.db.Exec(q)
		if err != nil {
			return err
		}
	}
	return nil
}
