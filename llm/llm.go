package llm

type Message struct {
	Role    string
	Content string
}

type Request struct {
	Model    string
	Messages []Message
}

type Response struct {
	Content string
}

type Provider interface {
	Chat(req Request) (Response, error)
}
