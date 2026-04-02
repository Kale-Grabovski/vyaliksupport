package webhook

// MessageCreatedEvent represents the webhook payload for message_created event.
type MessageCreatedEvent struct {
	Event        string `json:"event"`
	ID           int    `json:"id"`
	Content      string `json:"content"`
	MessageType  string `json:"message_type"`
	Conversation struct {
		ID      int `json:"id"`
		InboxID int `json:"inbox_id"`
		Attrs   struct {
			TgID int64 `json:"chat_id"`
		} `json:"additional_attributes"`
	} `json:"conversation"`
	Sender struct {
		Attrs struct {
			Username string `json:"username"`
		} `json:"additional_attributes"`
	} `json:"sender"`
	Account struct {
		ID int `json:"id"`
	} `json:"account"`
}
