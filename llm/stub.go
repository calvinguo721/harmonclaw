// Package llm (stub) provides StubProvider when LLM is not configured.
package llm

// StubProvider returns a fixed response when LLM is not configured.
type StubProvider struct{}

func (s *StubProvider) Chat(req Request) (Response, error) {
	return Response{Content: "[stub] LLM not configured. Set DEEPSEEK_API_KEY."}, nil
}
