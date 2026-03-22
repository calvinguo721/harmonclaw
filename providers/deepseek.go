package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DeepSeekAPIURL       = "https://api.deepseek.com/v1/chat/completions"
	DeepSeekDefaultModel = "deepseek-chat"
)

// DeepSeekProvider calls DeepSeek's OpenAI-compatible API.
type DeepSeekProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewDeepSeekProvider builds a DeepSeek provider. httpClient should be governor.SecureClient().
func NewDeepSeekProvider(apiKey string, client *http.Client) *DeepSeekProvider {
	return &DeepSeekProvider{
		apiKey:     apiKey,
		baseURL:    DeepSeekAPIURL,
		model:      DeepSeekDefaultModel,
		httpClient: client,
	}
}

func (d *DeepSeekProvider) Name() string { return "deepseek" }

func (d *DeepSeekProvider) Available() bool { return d.apiKey != "" }

func (d *DeepSeekProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = d.model
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deepseek: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepseek: status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage Usage  `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("deepseek: decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("deepseek: empty response")
	}

	return &ChatResponse{
		Content:      apiResp.Choices[0].Message.Content,
		ToolCalls:    apiResp.Choices[0].Message.ToolCalls,
		Model:        apiResp.Model,
		Usage:        apiResp.Usage,
		FinishReason: apiResp.Choices[0].FinishReason,
	}, nil
}

// ChatStream streams completion deltas (OpenAI SSE format).
func (d *DeepSeekProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan string, error) {
	if !d.Available() {
		return nil, fmt.Errorf("deepseek: not available")
	}
	model := req.Model
	if model == "" {
		model = d.model
	}
	body := map[string]any{
		"model":    model,
		"messages": req.Messages,
		"stream":   true,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.Temperature != 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("deepseek: marshal stream request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("deepseek: create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	httpResp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek: stream request: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, fmt.Errorf("deepseek: stream status %d", httpResp.StatusCode)
	}

	ch := make(chan string, 8)
	go func() {
		defer httpResp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(httpResp.Body)
		sc.Buffer(nil, 64*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var evt struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &evt) != nil {
				continue
			}
			if len(evt.Choices) > 0 && evt.Choices[0].Delta.Content != "" {
				ch <- evt.Choices[0].Delta.Content
			}
		}
	}()
	return ch, nil
}
