// Package channel provides WebSocket transport (placeholder).
package channel

import (
	"context"
	"net/http"
)

const wsChannelID = "websocket"

// WSChannel is a placeholder for WebSocket transport.
type WSChannel struct{}

// NewWSChannel creates a WebSocket channel placeholder.
func NewWSChannel() *WSChannel {
	return &WSChannel{}
}

// ID returns "websocket".
func (w *WSChannel) ID() string {
	return wsChannelID
}

// Receive returns nil; WebSocket upgrade handled in ServeHTTP.
func (w *WSChannel) Receive(ctx context.Context) (<-chan InboundMessage, error) {
	return nil, nil
}

// Send is a placeholder.
func (w *WSChannel) Send(ctx context.Context, msg OutboundMessage) error {
	return nil
}

// ServeHTTP is a placeholder; upgrade to WS in gateway/websocket.go.
func (w *WSChannel) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	http.Error(rw, "websocket upgrade not implemented", http.StatusNotImplemented)
}
