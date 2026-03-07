// Package engine provides the response generation pipeline.
package engine

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const (
	maxResponseLen = 4096
	fallbackReply  = "抱歉，服务暂时不可用，请稍后再试。"
)

// LLMCaller invokes the LLM.
type LLMCaller interface {
	Chat(messages []Message) (string, error)
	ChatStream(messages []Message) (<-chan string, error)
}

// SkillExecutor executes a skill and returns the result.
type SkillExecutor interface {
	Execute(skillID, text string, args map[string]string) (string, error)
}

// SkillRouter routes user text to skill IDs.
type SkillRouter interface {
	Route(text string) []string
}

// ResponsePipeline orchestrates intent → context → LLM/skill → post-process → memory.
type ResponsePipeline struct {
	intent    *IntentRecognizer
	context   *ContextManager
	llm       LLMCaller
	skill     SkillExecutor
	router    SkillRouter
	sensitive []string
	mu        sync.RWMutex
}

// Message for LLM.
type Message struct {
	Role    string
	Content string
}

// PipelineRequest is the input to the pipeline.
type PipelineRequest struct {
	SessionID string
	UserID    string
	Text      string
	Stream    bool
}

// PipelineResponse is the output.
type PipelineResponse struct {
	Content string
	Intent  string
	Stream  <-chan string
}

// NewResponsePipeline creates a pipeline.
func NewResponsePipeline(intent *IntentRecognizer, ctx *ContextManager, llm LLMCaller, skill SkillExecutor, router SkillRouter) *ResponsePipeline {
	p := &ResponsePipeline{
		intent:  intent,
		context: ctx,
		llm:     llm,
		skill:   skill,
		router:  router,
	}
	p.loadSensitiveWords("configs/sensitive_words.json")
	return p
}

// NewResponsePipelineWithSensitive creates a pipeline with explicit sensitive words (for tests).
func NewResponsePipelineWithSensitive(intent *IntentRecognizer, ctx *ContextManager, llm LLMCaller, skill SkillExecutor, router SkillRouter, sensitive []string) *ResponsePipeline {
	p := &ResponsePipeline{
		intent:    intent,
		context:   ctx,
		llm:       llm,
		skill:     skill,
		router:    router,
		sensitive: sensitive,
	}
	return p
}

func (p *ResponsePipeline) loadSensitiveWords(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		p.sensitive = nil
		return
	}
	json.Unmarshal(data, &p.sensitive)
}

// Run executes the full pipeline.
func (p *ResponsePipeline) Run(req PipelineRequest) (PipelineResponse, error) {
	res := p.intent.Recognize(req.Text, req.SessionID)

	p.context.Append(req.SessionID, "user", req.Text)

	if res.Intent == IntentSkill && p.skill != nil && p.router != nil {
		ids := p.router.Route(req.Text)
		if len(ids) > 0 {
			out, err := p.skill.Execute(ids[0], req.Text, nil)
			if err != nil {
				out = fallbackReply
			}
			out = p.postProcess(out)
			p.context.Append(req.SessionID, "assistant", out)
			return PipelineResponse{Content: out, Intent: res.Intent}, nil
		}
	}

	if res.Intent == IntentMemory {
		turns, mems := p.context.GetContextForLLM(req.SessionID, req.Text)
		prompt := FormatContextForPrompt(turns, mems)
		msgs := []Message{{Role: "user", Content: "根据以下记忆总结用户询问的内容：\n\n" + prompt + "\n\n用户问：" + req.Text}}
		if p.llm != nil {
			out, err := p.llm.Chat(msgs)
			if err != nil {
				out = fallbackReply
			}
			out = p.postProcess(out)
			p.context.Append(req.SessionID, "assistant", out)
			return PipelineResponse{Content: out, Intent: res.Intent}, nil
		}
		return PipelineResponse{Content: fallbackReply, Intent: res.Intent}, nil
	}

	if res.Intent == IntentSystem {
		return PipelineResponse{Content: "系统状态正常。", Intent: res.Intent}, nil
	}

	turns, mems := p.context.GetContextForLLM(req.SessionID, req.Text)
	msgs := buildMessages(turns, mems, req.Text)

	if req.Stream && p.llm != nil {
		ch, err := p.llm.ChatStream(msgs)
		if err != nil {
			return PipelineResponse{Content: fallbackReply, Intent: res.Intent}, nil
		}
		return PipelineResponse{Intent: res.Intent, Stream: ch}, nil
	}

	if p.llm != nil {
		out, err := p.llm.Chat(msgs)
		if err != nil {
			out = fallbackReply
		}
		out = p.postProcess(out)
		p.context.Append(req.SessionID, "assistant", out)
		return PipelineResponse{Content: out, Intent: res.Intent}, nil
	}

	return PipelineResponse{Content: fallbackReply, Intent: res.Intent}, nil
}

func buildMessages(turns []Turn, mems []string, last string) []Message {
	var msgs []Message
	if len(mems) > 0 {
		msgs = append(msgs, Message{Role: "system", Content: "Relevant memories:\n" + strings.Join(mems, "\n")})
	}
	for _, t := range turns {
		msgs = append(msgs, Message{Role: t.Role, Content: t.Content})
	}
	msgs = append(msgs, Message{Role: "user", Content: last})
	return msgs
}

func (p *ResponsePipeline) postProcess(s string) string {
	p.mu.RLock()
	words := p.sensitive
	p.mu.RUnlock()

	for _, w := range words {
		s = strings.ReplaceAll(s, w, "***")
	}
	if len(s) > maxResponseLen {
		s = s[:maxResponseLen] + "..."
	}
	return strings.TrimSpace(s)
}
