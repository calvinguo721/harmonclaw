// Package tts provides EdgeTTS and PiperTTS synthesis.
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
	"os/exec"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&TTS{})
}

const defaultVoice = "zh-CN-XiaoyiNeural"

type TTSBackend interface {
	Synthesize(text, voice string) ([]byte, error)
}

type EdgeTTS struct {
	apiURL string
	client *http.Client
}

func NewEdgeTTS() *EdgeTTS {
	url := os.Getenv("EDGE_TTS_URL")
	if url == "" {
		url = "http://localhost:5000/synthesize"
	}
	return &EdgeTTS{apiURL: url, client: governor.SecureClient()}
}

func (e *EdgeTTS) Synthesize(text, voice string) ([]byte, error) {
	if voice == "" {
		voice = defaultVoice
	}
	body, _ := json.Marshal(map[string]string{"text": text, "voice": voice})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("edge tts %d: %s", resp.StatusCode, b)
	}
	return io.ReadAll(resp.Body)
}

type PiperTTS struct {
	modelPath string
}

func NewPiperTTS() *PiperTTS {
	path := os.Getenv("PIPER_MODEL")
	if path == "" {
		path = "zh_CN-huayan-medium.onnx"
	}
	return &PiperTTS{modelPath: path}
}

func (p *PiperTTS) Synthesize(text, _ string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "piper-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("piper", "--model", p.modelPath, "--output_file", tmpPath)
	cmd.Stdin = bytes.NewReader([]byte(text))
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("piper: %w: %s", err, out)
	}
	return os.ReadFile(tmpPath)
}

type TTS struct{}

func (t *TTS) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "tts", Version: "0.1.0", Core: "architect"}
}

func (t *TTS) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx := context.Background()
	return skills.RunSandboxed(ctx, input.TraceID, func() skills.SkillOutput {
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

	sovereignty := "airlock"
	if input.Args != nil && input.Args["sovereignty"] != "" {
		sovereignty = input.Args["sovereignty"]
	}

	var backend TTSBackend
	if sovereignty == "shadow" {
		backend = NewPiperTTS()
	} else {
		backend = NewEdgeTTS()
	}

	audio, err := backend.Synthesize(text, voice)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}

	data, _ := json.Marshal(map[string]any{"audio_base64": base64.StdEncoding.EncodeToString(audio), "bytes": len(audio)})
	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(audio)
	return out
}
