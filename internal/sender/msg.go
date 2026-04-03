package sender

import "context"

type Message struct {
	Title    string
	Body     string
	Priority string
	Tags     []string
	Click    string
	Markdown bool
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}
