// Package llm defines Provider interface and message types.
package llm

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Response struct {
	Content string `json:"content"`
}

type Provider interface {
	Chat(req Request) (Response, error)
	ChatStream(req Request) (<-chan string, error)
}
