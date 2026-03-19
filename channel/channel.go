// Package channel defines pluggable transport interfaces (HTTP / WebSocket / WeChat).
package channel

import (
	"context"
	"net/http"
)

// InboundMessage is a message from a channel.
type InboundMessage struct {
	ChannelID string
	UserID    string
	SessionID string
	Text      string
	Raw       []byte
}

// OutboundMessage is a message to send to a channel.
type OutboundMessage struct {
	ChannelID string
	UserID    string
	Text      string
	Stream    bool
}

// Channel is a pluggable transport for receiving and sending messages.
type Channel interface {
	ID() string
	Receive(ctx context.Context) (<-chan InboundMessage, error)
	Send(ctx context.Context, msg OutboundMessage) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// ChannelRegistry holds registered channels.
var Registry = make(map[string]Channel)

// Register adds a channel.
func Register(c Channel) {
	Registry[c.ID()] = c
}
