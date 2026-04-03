package webhook

import "time"

// Attachment represents a file attachment in Chatwoot messages.
type Attachment struct {
	ID          int    `json:"id"`
	FileType    string `json:"file_type"`
	ContentType string `json:"content_type"`
	FileName    string `json:"file_name"`
	FileSize    int    `json:"file_size"`
	URL         string `json:"url"`
	ThumbURL    string `json:"thumb_url,omitempty"`
}

// MessageCreatedEvent represents the webhook payload for message_created event.
type MessageCreatedEvent struct {
	Event        string       `json:"event"`
	ID           int          `json:"id"`
	Content      string       `json:"content"`
	MessageType  string       `json:"message_type"`
	ContentType  string       `json:"content_type"`
	Attachments  []Attachment `json:"attachments"`
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

// AutoMessageCreatedEvent represents the webhook payload for message_created event sent from Settings -> Automation menu.
type AutoMessageCreatedEvent struct {
	AdditionalAttributes struct {
		ChatId               int         `json:"chat_id"`
		BusinessConnectionId interface{} `json:"business_connection_id"`
	} `json:"additional_attributes"`
	CanReply     bool   `json:"can_reply"`
	Channel      string `json:"channel"`
	ContactInbox struct {
		Id           int       `json:"id"`
		ContactId    int       `json:"contact_id"`
		InboxId      int       `json:"inbox_id"`
		SourceId     string    `json:"source_id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		HmacVerified bool      `json:"hmac_verified"`
		PubsubToken  string    `json:"pubsub_token"`
	} `json:"contact_inbox"`
	Id       int `json:"id"`
	InboxId  int `json:"inbox_id"`
	Messages []struct {
		Id                int       `json:"id"`
		Content           string    `json:"content"`
		AccountId         int       `json:"account_id"`
		InboxId           int       `json:"inbox_id"`
		ConversationId    int       `json:"conversation_id"`
		MessageType       int       `json:"message_type"`
		CreatedAt         int       `json:"created_at"`
		UpdatedAt         time.Time `json:"updated_at"`
		Private           bool      `json:"private"`
		Status            string    `json:"status"`
		SourceId          string    `json:"source_id"`
		ContentType       string    `json:"content_type"`
		ContentAttributes struct {
		} `json:"content_attributes"`
		SenderType        string `json:"sender_type"`
		SenderId          int    `json:"sender_id"`
		ExternalSourceIds struct {
		} `json:"external_source_ids"`
		AdditionalAttributes struct {
		} `json:"additional_attributes"`
		ProcessedMessageContent string `json:"processed_message_content"`
		Sentiment               struct {
		} `json:"sentiment"`
		Attachments  []Attachment `json:"attachments"`
		Conversation struct {
			AssigneeId     int `json:"assignee_id"`
			UnreadCount    int `json:"unread_count"`
			LastActivityAt int `json:"last_activity_at"`
			ContactInbox   struct {
				SourceId string `json:"source_id"`
			} `json:"contact_inbox"`
		} `json:"conversation"`
		Sender struct {
			AdditionalAttributes struct {
				Username               string `json:"username"`
				LanguageCode           string `json:"language_code"`
				SocialTelegramUserId   int    `json:"social_telegram_user_id"`
				SocialTelegramUserName string `json:"social_telegram_user_name"`
			} `json:"additional_attributes"`
			CustomAttributes struct {
			} `json:"custom_attributes"`
			Email       interface{} `json:"email"`
			Id          int         `json:"id"`
			Identifier  interface{} `json:"identifier"`
			Name        string      `json:"name"`
			PhoneNumber interface{} `json:"phone_number"`
			Thumbnail   string      `json:"thumbnail"`
			Blocked     bool        `json:"blocked"`
			Type        string      `json:"type"`
		} `json:"sender"`
	} `json:"messages"`
	Labels []interface{} `json:"labels"`
	Meta   struct {
		Sender struct {
			AdditionalAttributes struct {
				Username               string `json:"username"`
				LanguageCode           string `json:"language_code"`
				SocialTelegramUserId   int64  `json:"social_telegram_user_id"`
				SocialTelegramUserName string `json:"social_telegram_user_name"`
			} `json:"additional_attributes"`
			CustomAttributes struct {
			} `json:"custom_attributes"`
			Email       interface{} `json:"email"`
			Id          int         `json:"id"`
			Identifier  interface{} `json:"identifier"`
			Name        string      `json:"name"`
			PhoneNumber interface{} `json:"phone_number"`
			Thumbnail   string      `json:"thumbnail"`
			Blocked     bool        `json:"blocked"`
			Type        string      `json:"type"`
		} `json:"sender"`
		Assignee struct {
			Id                 int         `json:"id"`
			Name               string      `json:"available_name"`
			AvatarUrl          string      `json:"avatar_url"`
			Type               string      `json:"type"`
			AvailabilityStatus interface{} `json:"availability_status"`
			Thumbnail          string      `json:"thumbnail"`
		} `json:"assignee"`
		AssigneeType string      `json:"assignee_type"`
		Team         interface{} `json:"team"`
		HmacVerified bool        `json:"hmac_verified"`
	} `json:"meta"`
	Status           string `json:"status"`
	CustomAttributes struct {
	} `json:"custom_attributes"`
	SnoozedUntil        interface{} `json:"snoozed_until"`
	UnreadCount         int         `json:"unread_count"`
	FirstReplyCreatedAt time.Time   `json:"first_reply_created_at"`
	Priority            interface{} `json:"priority"`
	WaitingSince        int         `json:"waiting_since"`
	AgentLastSeenAt     int         `json:"agent_last_seen_at"`
	ContactLastSeenAt   int         `json:"contact_last_seen_at"`
	LastActivityAt      int         `json:"last_activity_at"`
	Timestamp           int         `json:"timestamp"`
	CreatedAt           int         `json:"created_at"`
	UpdatedAt           float64     `json:"updated_at"`
	Event               string      `json:"event"`
}
