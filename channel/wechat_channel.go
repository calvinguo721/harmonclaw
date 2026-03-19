// Package channel provides WeChat transport (placeholder).
package channel

import (
	"context"
	"net/http"
)

const wechatChannelID = "wechat"

// WeChatChannel is a placeholder for WeChat integration.
type WeChatChannel struct{}

// NewWeChatChannel creates a WeChat channel placeholder.
func NewWeChatChannel() *WeChatChannel {
	return &WeChatChannel{}
}

// ID returns "wechat".
func (w *WeChatChannel) ID() string {
	return wechatChannelID
}

// Receive returns nil; WeChat webhook handled in ServeHTTP.
func (w *WeChatChannel) Receive(ctx context.Context) (<-chan InboundMessage, error) {
	return nil, nil
}

// Send is a placeholder.
func (w *WeChatChannel) Send(ctx context.Context, msg OutboundMessage) error {
	return nil
}

// ServeHTTP is a placeholder for WeChat callback verification and message handling.
func (w *WeChatChannel) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	http.Error(rw, "wechat integration not implemented", http.StatusNotImplemented)
}
