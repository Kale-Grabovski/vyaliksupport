package mysql

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
	query := "INSERT INTO tg_support_requests (support_message_id, user_chat_id) VALUES (?, ?)"
	_, err := d.db.Exec(query, supportMessageID, userChatID)
	return err
}

func (d *Req) FindUserChatID(supportMessageID int) (userChatID int64, err error) {
	query := "SELECT user_chat_id FROM tg_support_requests WHERE support_message_id = ?"
	err = d.db.QueryRow(query, supportMessageID).Scan(&userChatID)
	return
}

func (d *Req) Migrate() error {
	query := `
		CREATE TABLE IF NOT EXISTS tg_support_requests (
			id int AUTO_INCREMENT PRIMARY KEY,
			support_message_id int NOT NULL,
			user_chat_id bigint NOT NULL,
			created_at datetime DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_support_message_id (support_message_id),
			INDEX idx_user_chat_id (user_chat_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`
	_, err := d.db.Exec(query)
	return err
}
