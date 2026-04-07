package domain

import (
	"encoding/json"
	"time"
)

const (
	DirectionToGroup = "to_group"
	DirectionToUser  = "to_user"
	DirectionAck     = "ack" // acknowledgment with group_message_id
)

const (
	ContentTypeText      = "text"
	ContentTypePhoto     = "photo"
	ContentTypeVideo     = "video"
	ContentTypeSticker   = "sticker"
	ContentTypeDocument  = "document"
	ContentTypeAudio     = "audio"
	ContentTypeVoice     = "voice"
	ContentTypeAnimation = "animation"
)

// Payload is the message structure exchanged between bot and group via ntfy.
type Payload struct {
	Direction        string    `json:"direction"` // "to_group" | "to_user" | "ack"
	UserChatID       int64     `json:"user_chat_id"`
	MsgID            int       `json:"msg_id"`             // message ID in bot (for reply)
	GroupMsgID       int       `json:"group_msg_id"`       // message ID in group (for reply lookup)
	SupportGroupChat int64     `json:"support_group_chat"` // group chat ID for copy operations
	DownloadURL      string    `json:"download_url"`       // URL from file.io for media transfer
	Summary          string    `json:"summary"`            // user summary
	Content          Content   `json:"content"`            // text/media
	CreatedAt        time.Time `json:"created_at"`
}

// Content holds the message content (text or media).
type Content struct {
	Type     string `json:"type"`      // "text", "photo", "video", etc.
	Text     string `json:"text"`      // Text content
	FileID   string `json:"file_id"`   // TG file_id
	Caption  string `json:"caption"`   // Media caption
	FileName string `json:"file_name"` // Document filename
}

// Marshal serializes Payload to JSON.
func (p *Payload) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// UnmarshalPayload deserializes JSON to Payload.
func UnmarshalPayload(data []byte) (*Payload, error) {
	var p Payload
	err := json.Unmarshal(data, &p)
	return &p, err
}
