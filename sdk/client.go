// Package sdk provides Go client for HarmonClaw API.
package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client is the HarmonClaw API client.
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a client. token can be empty for unauthenticated endpoints.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
	}
}

func (c *Client) req(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

// Chat sends a chat message.
func (c *Client) Chat(msg string) (map[string]any, error) {
	resp, err := c.req("POST", "/v1/chat/completions", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": msg}},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil, fmt.Errorf("chat: %d", resp.StatusCode)
	}
	return out, nil
}

// Search performs Viking search.
func (c *Client) Search(query string) (map[string]any, error) {
	resp, err := c.req("GET", "/v1/viking/search?q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil, fmt.Errorf("search: %d", resp.StatusCode)
	}
	return out, nil
}

// Skills returns registered skills.
func (c *Client) Skills() (map[string]any, error) {
	resp, err := c.req("GET", "/v1/architect/skills", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil, fmt.Errorf("skills: %d", resp.StatusCode)
	}
	return out, nil
}

// Health returns health status.
func (c *Client) Health() (map[string]any, error) {
	resp, err := c.req("GET", "/v1/health", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil, fmt.Errorf("health: %d", resp.StatusCode)
	}
	return out, nil
}
