package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"harmonclaw/configs"
	"harmonclaw/governor"
	"harmonclaw/providers"
	"harmonclaw/skills"
)

const maxToolRounds = 6

type routerLLM struct {
	router   *providers.Router
	streamer *providers.DeepSeekProvider
}

// NewProvider returns a Router-backed DeepSeek implementation, or StubProvider when no API key.
func NewProvider() (Provider, error) {
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		if c := configs.Get(); c != nil {
			key = strings.TrimSpace(c.DeepSeekAPIKey)
		}
	}
	if key == "" {
		return &StubProvider{}, nil
	}
	client := governor.SecureClient()
	r := providers.NewRouter("deepseek")
	ds := providers.NewDeepSeekProvider(key, client)
	r.Register(ds)
	return &routerLLM{router: r, streamer: ds}, nil
}

func (p *routerLLM) Chat(req Request) (Response, error) {
	ctx := context.Background()
	msgs := toProviderMessages(req.Messages)
	cr := &providers.ChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
	}
	if skills.BraveSearchConfigured() {
		cr.Tools = webSearchTools()
	}

	for round := 0; round < maxToolRounds; round++ {
		if round > 0 {
			cr.Tools = nil
		}
		resp, err := p.router.Chat(ctx, cr)
		if err != nil {
			return Response{}, err
		}
		if len(resp.ToolCalls) == 0 {
			return Response{Content: resp.Content}, nil
		}

		cr.Messages = append(cr.Messages, providers.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			if tc.Type != "" && tc.Type != "function" {
				continue
			}
			name := strings.TrimSpace(tc.Function.Name)
			toolID := tc.ID
			payload, terr := runToolCall(name, tc.Function.Arguments)
			if terr != nil {
				payload = fmt.Sprintf(`{"error":%q}`, terr.Error())
			}
			cr.Messages = append(cr.Messages, providers.ChatMessage{
				Role:       "tool",
				ToolCallID: toolID,
				Content:    payload,
			})
		}
	}

	return Response{}, fmt.Errorf("llm: tool loop exceeded %d rounds", maxToolRounds)
}

func (p *routerLLM) ChatStream(req Request) (<-chan string, error) {
	ctx := context.Background()
	cr := &providers.ChatRequest{
		Model:    req.Model,
		Messages: toProviderMessages(req.Messages),
		Stream:   true,
	}
	return p.router.ChatStream(ctx, cr)
}

func toProviderMessages(msgs []Message) []providers.ChatMessage {
	out := make([]providers.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, providers.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

func webSearchTools() []providers.Tool {
	return []providers.Tool{{
		Type: "function",
		Function: providers.ToolFunction{
			Name:        "web_search",
			Description: "Search the public web for up-to-date facts, news, or topics. Use when the user asks for current information.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Concise search query in the user's language",
					},
				},
				"required": []string{"query"},
			},
		},
	}}
}

func runToolCall(name, arguments string) (string, error) {
	switch strings.TrimSpace(name) {
	case "web_search":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &args); err != nil {
			return "", fmt.Errorf("web_search args: %w", err)
		}
		q := strings.TrimSpace(args.Query)
		if q == "" {
			return "", fmt.Errorf("web_search: empty query")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := skills.BraveSearchNormalizedJSON(ctx, q, 0)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}
