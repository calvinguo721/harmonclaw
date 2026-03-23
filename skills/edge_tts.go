// Package skills — Edge TTS (Microsoft neural speech) via github.com/difyz9/edge-tts-go.
package skills

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/difyz9/edge-tts-go/pkg/communicate"

	"harmonclaw/governor"
)

func init() {
	Register(NewEdgeTTSSkill(""))
}

// EdgeTTSWSSHost is the hostname used for Edge TTS WebSocket (must match sovereignty whitelist).
const EdgeTTSWSSHost = "api.msedgeservices.com"

// Default voice IDs (Edge neural)
const (
	DefaultVoiceZH = "zh-CN-XiaoxiaoNeural"
	DefaultVoiceEN = "en-US-AriaNeural"
)

// EdgeTTSConfigFile is loaded from configs/edge_tts.json (optional).
type EdgeTTSConfigFile struct {
	DefaultVoice string            `json:"default_voice"`
	Voices       map[string]string `json:"voices"`
}

// EdgeTTSAliases returns a copy of voice aliases from configs/edge_tts.json (for HTTP handlers).
func EdgeTTSAliases() map[string]string {
	cfg := loadEdgeTTSConfigFile()
	out := make(map[string]string, len(cfg.Voices))
	for k, v := range cfg.Voices {
		out[k] = v
	}
	return out
}

func loadEdgeTTSConfigFile() EdgeTTSConfigFile {
	c := EdgeTTSConfigFile{
		DefaultVoice: DefaultVoiceZH,
		Voices:       map[string]string{},
	}
	path := strings.TrimSpace(os.Getenv("HC_EDGE_TTS_CONFIG"))
	if path == "" {
		path = filepath.Join("configs", "edge_tts.json")
	}
	paths := []string{path}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, path))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if json.Unmarshal(data, &c) == nil {
			if c.DefaultVoice == "" {
				c.DefaultVoice = DefaultVoiceZH
			}
			break
		}
	}
	return c
}

// EdgeTTSSkill synthesizes MP3 using Edge TTS (WebSocket to Microsoft).
type EdgeTTSSkill struct {
	defaultVoice string
	cfg          EdgeTTSConfigFile
}

// NewEdgeTTSSkill returns a skill; if defaultVoice is empty, uses config file / DefaultVoiceZH.
func NewEdgeTTSSkill(defaultVoice string) *EdgeTTSSkill {
	cfg := loadEdgeTTSConfigFile()
	v := strings.TrimSpace(defaultVoice)
	if v == "" {
		v = cfg.DefaultVoice
	}
	if v == "" {
		v = DefaultVoiceZH
	}
	return &EdgeTTSSkill{defaultVoice: v, cfg: cfg}
}

func (e *EdgeTTSSkill) GetIdentity() SkillIdentity {
	return SkillIdentity{ID: "edge_tts", Version: "0.1.0", Core: "architect"}
}

func (e *EdgeTTSSkill) Execute(input SkillInput) SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return RunSandboxedWithTimeout(ctx, input.TraceID, 90*time.Second, func() SkillOutput {
		return e.doExecute(ctx, input)
	})
}

func (e *EdgeTTSSkill) doExecute(ctx context.Context, input SkillInput) SkillOutput {
	text := strings.TrimSpace(input.Text)
	if text == "" && input.Args != nil {
		text = strings.TrimSpace(input.Args["text"])
	}
	if text == "" {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: "text is empty"}
	}
	voice := e.defaultVoice
	if input.Args != nil {
		if v := strings.TrimSpace(input.Args["voice"]); v != "" {
			voice = MapOpenAIOrAliasVoice(v, e.cfg.Voices)
		}
	}
	data, err := e.Synthesize(ctx, text, voice)
	if err != nil {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	type out struct {
		Format string `json:"format"`
		Data   string `json:"data"`
	}
	raw, _ := json.Marshal(out{Format: "mp3", Data: base64.StdEncoding.EncodeToString(data)})
	return SkillOutput{TraceID: input.TraceID, Status: "ok", Data: raw}
}

// Synthesize returns MP3 bytes. Enforces sovereignty allowlist for Edge TTS host before dialing.
func (e *EdgeTTSSkill) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("edge_tts: empty text")
	}
	if err := governor.AllowOutboundHost(EdgeTTSWSSHost); err != nil {
		return nil, err
	}
	if strings.TrimSpace(voice) == "" {
		voice = e.defaultVoice
	}
	comm, err := communicate.NewCommunicate(text, voice, "+0%", "+0%", "+0Hz", "", 15, 90)
	if err != nil {
		return nil, fmt.Errorf("edge_tts: %w", err)
	}
	chunkCh, errCh := comm.Stream(ctx)
	var buf bytes.Buffer
	for chunk := range chunkCh {
		if chunk.Type == "audio" {
			buf.Write(chunk.Data)
		}
	}
	if err := <-errCh; err != nil {
		return nil, fmt.Errorf("edge_tts: stream: %w", err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("edge_tts: no audio received")
	}
	return buf.Bytes(), nil
}

// SynthesizeToWriter writes MP3 to w.
func (e *EdgeTTSSkill) SynthesizeToWriter(ctx context.Context, text, voice string, w io.Writer) error {
	data, err := e.Synthesize(ctx, text, voice)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// MapOpenAIOrAliasVoice maps OpenAI-style names, config aliases, or passes through Edge voice IDs.
func MapOpenAIOrAliasVoice(voice string, aliases map[string]string) string {
	v := strings.TrimSpace(voice)
	if v == "" {
		return ""
	}
	if aliases != nil {
		if mapped, ok := aliases[v]; ok && strings.TrimSpace(mapped) != "" {
			return mapped
		}
	}
	switch v {
	case "alloy", "nova", "shimmer":
		return DefaultVoiceZH
	case "echo", "fable", "onyx":
		return "zh-CN-YunxiNeural"
	default:
		return v
	}
}
