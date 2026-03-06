// Package llm (deepseek) provides DeepSeek API client.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"harmonclaw/governor"
)

const deepseekURL = "https://api.deepseek.com/v1/chat/completions"

type apiChoice struct {
	Message Message `json:"message"`
}

type apiResponse struct {
	Choices []apiChoice `json:"choices"`
	Error   *apiError   `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
}

type DeepSeekClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

// NewProvider returns DeepSeek client or StubProvider when API key is not set.
func NewProvider() (Provider, error) {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		return &StubProvider{}, nil
	}
	c, err := NewDeepSeekClient()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func NewDeepSeekClient() (*DeepSeekClient, error) {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("environment variable DEEPSEEK_API_KEY is not set")
	}
	return &DeepSeekClient{
		apiKey:     key,
		endpoint:   deepseekURL,
		httpClient: governor.SecureClient(),
	}, nil
}

func (c *DeepSeekClient) Chat(req Request) (Response, error) {
	if req.Model == "" {
		req.Model = "deepseek-chat"
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("api returned status %d: %s", httpResp.StatusCode, respBody)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return Response{}, fmt.Errorf("api returned zero choices")
	}

	return Response{Content: apiResp.Choices[0].Message.Content}, nil
}
