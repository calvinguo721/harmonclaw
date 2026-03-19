// Package channel provides HTTP transport implementation.
package channel

import (
	"context"
	"encoding/json"
	"net/http"
)

const httpChannelID = "http"

// HTTPChannel implements Channel for HTTP request/response.
type HTTPChannel struct {
	inbound chan InboundMessage
}

// NewHTTPChannel creates an HTTP channel.
func NewHTTPChannel() *HTTPChannel {
	return &HTTPChannel{
		inbound: make(chan InboundMessage, 64),
	}
}

// ID returns "http".
func (h *HTTPChannel) ID() string {
	return httpChannelID
}

// Receive returns the inbound message channel.
func (h *HTTPChannel) Receive(ctx context.Context) (<-chan InboundMessage, error) {
	return h.inbound, nil
}

// Send writes response; for HTTP, caller typically handles response in handler.
func (h *HTTPChannel) Send(ctx context.Context, msg OutboundMessage) error {
	return nil
}

// ServeHTTP handles POST /message and pushes to inbound.
func (h *HTTPChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text string `json:"text"`
		User string `json:"user_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user := req.User
	if user == "" {
		user = "anonymous"
	}
	select {
	case h.inbound <- InboundMessage{
		ChannelID: httpChannelID,
		UserID:    user,
		SessionID: r.RemoteAddr,
		Text:      req.Text,
	}:
	default:
	}
	w.WriteHeader(http.StatusAccepted)
}
