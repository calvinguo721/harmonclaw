// Package gateway provides structured request logging with rotation.
package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logDir        = "logs"
	logMaxBytes   = 100 * 1024 * 1024 // 100MB
	logRotateDays = 1
)

var (
	logMu     sync.Mutex
	logFile   *os.File
	logSize   int64
	logDay    int
	redactKeys = map[string]bool{"password": true, "token": true, "authorization": true}
)

type accessLog struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	ActionID  string `json:"action_id"`
	UserID    string `json:"user_id"`
	ClientIP  string `json:"client_ip"`
	Timestamp string `json:"timestamp"`
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		latency := time.Since(start).Milliseconds()
		actionID := GetActionID(r.Context())
		userID := "anonymous"
		if ah := r.Header.Get("Authorization"); ah != "" {
			userID = "authenticated"
		}
		clientIP := r.RemoteAddr
		if idx := strings.LastIndex(clientIP, ":"); idx >= 0 {
			clientIP = clientIP[:idx]
		}
		writeAccessLog(accessLog{
			Method:    r.Method,
			Path:      redactPath(r.URL.Path),
			Status:    rec.status,
			LatencyMs: latency,
			ActionID:  actionID,
			UserID:    userID,
			ClientIP:  clientIP,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func redactPath(path string) string {
	for k := range redactKeys {
		if strings.Contains(strings.ToLower(path), k) {
			return "[redacted]"
		}
	}
	return path
}

func writeAccessLog(entry accessLog) {
	logMu.Lock()
	defer logMu.Unlock()
	f, err := ensureLogFile()
	if err != nil {
		return
	}
	data, _ := json.Marshal(entry)
	line := string(data) + "\n"
	n, _ := f.WriteString(line)
	logSize += int64(n)
}

func ensureLogFile() (*os.File, error) {
	now := time.Now()
	day := now.YearDay()
	if logFile != nil && logDay == day && logSize < logMaxBytes {
		return logFile, nil
	}
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	os.MkdirAll(logDir, 0755)
	name := filepath.Join(logDir, fmt.Sprintf("access.%s.jsonl", now.Format("2006-01-02")))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	info, _ := f.Stat()
	logFile = f
	logSize = info.Size()
	logDay = day
	return logFile, nil
}
