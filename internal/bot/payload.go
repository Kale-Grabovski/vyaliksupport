package bot

import (
	"vyaliksupport/internal/domain"
)

const (
	DirectionToGroup = domain.DirectionToGroup
	DirectionToUser  = domain.DirectionToUser
	DirectionAck     = domain.DirectionAck
)

const (
	ContentTypeText      = domain.ContentTypeText
	ContentTypePhoto     = domain.ContentTypePhoto
	ContentTypeVideo     = domain.ContentTypeVideo
	ContentTypeSticker   = domain.ContentTypeSticker
	ContentTypeDocument  = domain.ContentTypeDocument
	ContentTypeAudio     = domain.ContentTypeAudio
	ContentTypeVoice     = domain.ContentTypeVoice
	ContentTypeAnimation = domain.ContentTypeAnimation
)

// Payload is an alias to domain.Payload for convenience.
type Payload = domain.Payload

// Content is an alias to domain.Content for convenience.
type Content = domain.Content
