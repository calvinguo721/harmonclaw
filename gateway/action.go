// Package gateway provides action_id context and request logging.
package gateway

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
)

type contextKey string

const actionIDKey contextKey = "action_id"

func NewActionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", b[0])
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func WithActionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, actionIDKey, id)
}

func GetActionID(ctx context.Context) string {
	if v, ok := ctx.Value(actionIDKey).(string); ok {
		return v
	}
	return ""
}

func Log(ctx context.Context, format string, args ...any) {
	aid := GetActionID(ctx)
	if aid != "" {
		log.Printf("[action_id=%s] "+format, append([]any{aid}, args...)...)
	} else {
		log.Printf(format, args...)
	}
}

func actionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aid := NewActionID()
		ctx := WithActionID(r.Context(), aid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
