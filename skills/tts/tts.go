// Package tts provides TTS synthesis via API (incl. Edge TTS proxy) or text+phoneme fallback.
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&TTS{})
}

const (
	defaultVoice   = "zh-CN-XiaoyiNeural"
	defaultTimeout = 60 * time.Second
	maxTextLen     = 5000
)

var sentenceSplitRe = regexp.MustCompile(`[。！？.!?]\s*|\n+`)

type ttsConfig struct {
	DefaultVoice   string `json:"default_voice"`
	CacheTTLSec    int    `json:"cache_ttl_sec"`
	MaxConcurrent  int    `json:"max_concurrent"`
	MaxTextLen     int    `json:"max_text_len"`
	EdgeMode       bool   `json:"edge_mode"`
}

func loadTTSConfig() ttsConfig {
	cfg := ttsConfig{
		DefaultVoice:  defaultVoice,
		CacheTTLSec:   3600,
		MaxConcurrent: 2,
		MaxTextLen:    maxTextLen,
		EdgeMode:      false,
	}
	paths := []string{"configs/tts.json"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, "configs/tts.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, &cfg)
		if cfg.MaxTextLen <= 0 {
			cfg.MaxTextLen = maxTextLen
		}
		break
	}
	return cfg
}

type TTS struct{}

func (t *TTS) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "tts", Version: "0.3.0", Core: "architect"}
}

func (t *TTS) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, defaultTimeout, func() skills.SkillOutput {
		return t.doExecute(input)
	})
}

func (t *TTS) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	text := input.Text
	if text == "" && input.Args != nil {
		text = input.Args["text"]
	}
	if text == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "text is empty"}
	}

	cfg := loadTTSConfig()
	if len([]rune(text)) > cfg.MaxTextLen {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "text exceeds max length"}
	}

	voice := cfg.DefaultVoice
	if input.Args != nil && input.Args["voice"] != "" {
		voice = input.Args["voice"]
	}

	endpoint := strings.TrimSpace(os.Getenv("HC_TTS_ENDPOINT"))
	if endpoint != "" {
		cacheKey := ttsCacheKey(text, voice)
		if cached, ok := ttsCacheGet(cacheKey); ok {
			out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: cached}
			out.Metrics.Ms = time.Since(start).Milliseconds()
			out.Metrics.Bytes = len(cached)
			return out
		}
		if !ttsAcquire() {
			return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "tts concurrency limit exceeded"}
		}
		defer ttsRelease()

		out := t.synthesizeViaAPI(input, text, voice, endpoint, cfg.EdgeMode, start)
		if out.Status == "ok" && len(out.Data) > 0 {
			ttsCacheSet(cacheKey, out.Data)
		}
		return out
	}
	return t.fallbackTextPhoneme(input, text, start)
}

func (t *TTS) synthesizeViaAPI(input skills.SkillInput, text, voice, endpoint string, edgeMode bool, start time.Time) skills.SkillOutput {
	var req *http.Request
	var err error
	if edgeMode || os.Getenv("HC_TTS_EDGE_MODE") == "1" {
		form := url.Values{}
		form.Set("text", text)
		form.Set("voice", voice)
		req, err = http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		body, _ := json.Marshal(map[string]string{"text": text, "voice": voice})
		req, err = http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}

	client := governor.SecureClient()
	resp, err := client.Do(req)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: fmt.Sprintf("TTS API %d: %s", resp.StatusCode, b)}
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}

	data, _ := json.Marshal(map[string]any{
		"audio_base64": base64.StdEncoding.EncodeToString(audio),
		"bytes":        len(audio),
	})
	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(audio)
	return out
}

func (t *TTS) fallbackTextPhoneme(input skills.SkillInput, text string, start time.Time) skills.SkillOutput {
	sentences := sentenceSplitRe.Split(text, -1)
	var filtered []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{text}
	}

	phonemes := splitToPhonemes(text)

	data, _ := json.Marshal(map[string]any{
		"text":      text,
		"sentences": filtered,
		"phonemes":  phonemes,
		"fallback":  true,
	})
	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}

func splitToPhonemes(text string) []string {
	var out []string
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' {
			continue
		}
		out = append(out, string(r))
	}
	return out
}
