package engine

import (
	"errors"
	"strings"
	"testing"
)

type mockLLM struct {
	chat      string
	chatErr   error
	streamCh  chan string
	streamErr error
}

func (m *mockLLM) Chat(msgs []Message) (string, error) {
	if m.chatErr != nil {
		return "", m.chatErr
	}
	return m.chat, nil
}

func (m *mockLLM) ChatStream(msgs []Message) (<-chan string, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.streamCh, nil
}

type mockSkill struct {
	result string
	err    error
}

func (m *mockSkill) Execute(skillID, text string, args map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

type mockRouter struct {
	ids []string
}

func (m *mockRouter) Route(text string) []string {
	return m.ids
}

func TestResponsePipeline_SkillIntent(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	skill := &mockSkill{result: "search result"}
	router := &mockRouter{ids: []string{"web_search"}}
	pipe := NewResponsePipeline(intent, ctx, nil, skill, router)

	res, err := pipe.Run(PipelineRequest{SessionID: "s1", Text: "执行搜索 Go 语言", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != IntentSkill {
		t.Errorf("intent=%s (skill expected)", res.Intent)
	}
	if res.Content != "search result" {
		t.Errorf("content=%q", res.Content)
	}
}

func TestResponsePipeline_SystemIntent(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	pipe := NewResponsePipeline(intent, ctx, nil, nil, nil)

	res, err := pipe.Run(PipelineRequest{SessionID: "s2", Text: "系统状态", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != IntentSystem {
		t.Errorf("intent=%s", res.Intent)
	}
	if !strings.Contains(res.Content, "正常") {
		t.Errorf("content=%q", res.Content)
	}
}

func TestResponsePipeline_ChatWithLLM(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	llm := &mockLLM{chat: "你好！有什么可以帮你的？"}
	pipe := NewResponsePipeline(intent, ctx, llm, nil, nil)

	res, err := pipe.Run(PipelineRequest{SessionID: "s3", Text: "你好", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "你好！有什么可以帮你的？" {
		t.Errorf("content=%q", res.Content)
	}
}

func TestResponsePipeline_LLMFallback(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	llm := &mockLLM{chatErr: ErrLLMUnavailable}
	pipe := NewResponsePipeline(intent, ctx, llm, nil, nil)

	res, err := pipe.Run(PipelineRequest{SessionID: "s4", Text: "你好", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "不可用") && !strings.Contains(res.Content, "抱歉") {
		t.Errorf("expected fallback, got %q", res.Content)
	}
}

var ErrLLMUnavailable = errors.New("LLM unavailable")

func TestResponsePipeline_SensitiveFilter(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	llm := &mockLLM{chat: "这是 test_sensitive 内容"}
	pipe := NewResponsePipelineWithSensitive(intent, ctx, llm, nil, nil, []string{"test_sensitive"})

	res, err := pipe.Run(PipelineRequest{SessionID: "s5", Text: "测试", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "test_sensitive") {
		t.Errorf("sensitive word not filtered: %q", res.Content)
	}
	if !strings.Contains(res.Content, "***") {
		t.Errorf("expected *** replacement: %q", res.Content)
	}
}

func TestResponsePipeline_LengthTruncate(t *testing.T) {
	intent := NewIntentRecognizer("")
	ctx := NewContextManager(nil, nil)
	long := strings.Repeat("x", 5000)
	llm := &mockLLM{chat: long}
	pipe := NewResponsePipeline(intent, ctx, llm, nil, nil)

	res, err := pipe.Run(PipelineRequest{SessionID: "s6", Text: "hi", Stream: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) > maxResponseLen+10 {
		t.Errorf("len=%d > %d", len(res.Content), maxResponseLen)
	}
	if !strings.HasSuffix(res.Content, "...") {
		t.Errorf("expected ... suffix: %q", res.Content[len(res.Content)-10:])
	}
}
