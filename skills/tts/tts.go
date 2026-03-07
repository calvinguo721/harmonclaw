// Package tts provides TTS synthesis via API or text+phoneme fallback.
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
)

var sentenceSplitRe = regexp.MustCompile(`[。！？.!?]\s*|\n+`)

type TTS struct{}

func (t *TTS) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "tts", Version: "0.2.0", Core: "architect"}
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

	voice := defaultVoice
	if input.Args != nil && input.Args["voice"] != "" {
		voice = input.Args["voice"]
	}

	endpoint := strings.TrimSpace(os.Getenv("HC_TTS_ENDPOINT"))
	if endpoint != "" {
		return t.synthesizeViaAPI(input, text, voice, endpoint, start)
	}
	return t.fallbackTextPhoneme(input, text, start)
}

func (t *TTS) synthesizeViaAPI(input skills.SkillInput, text, voice, endpoint string, start time.Time) skills.SkillOutput {
	body, _ := json.Marshal(map[string]string{"text": text, "voice": voice})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

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
