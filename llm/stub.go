// Package llm (stub) provides StubProvider when LLM is not configured.
package llm

// StubProvider returns a fixed response when LLM is not configured.
type StubProvider struct{}

func (s *StubProvider) Chat(req Request) (Response, error) {
	return Response{Content: "[stub] LLM not configured. Set DEEPSEEK_API_KEY."}, nil
}

func (s *StubProvider) ChatStream(req Request) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "[stub] LLM not configured. Set DEEPSEEK_API_KEY."
	close(ch)
	return ch, nil
}
