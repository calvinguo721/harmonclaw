package engine

import (
	"context"
	"testing"

	"harmonclaw/llm"
)

type mockAssembler struct {
	load   func(userID, sessionID string) ([]llm.Message, error)
	append func(userID, sessionID, role, content string) ([]llm.Message, error)
}

func (m *mockAssembler) LoadContext(userID, sessionID string) ([]llm.Message, error) {
	if m.load != nil {
		return m.load(userID, sessionID)
	}
	return nil, nil
}

func (m *mockAssembler) Append(userID, sessionID, role, content string) ([]llm.Message, error) {
	if m.append != nil {
		return m.append(userID, sessionID, role, content)
	}
	return nil, nil
}

func TestResolveSessionKey(t *testing.T) {
	if k := ResolveSessionKey("main", "", "", ScopeMain); k != "agent:main:main" {
		t.Errorf("main: got %s", k)
	}
	if k := ResolveSessionKey("", "telegram", "123", ScopePerChannelPeer); k != "agent:main:telegram:dm:123" {
		t.Errorf("per-channel: got %s", k)
	}
}

func TestConversationEngine_StartTurn(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	a := &mockAssembler{
		load: func(_, _ string) ([]llm.Message, error) { return msgs, nil },
	}
	eng := NewConversationEngine(a)
	ctx := context.Background()

	runID, got, sub, err := eng.StartTurn(ctx, "u1", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if runID == "" {
		t.Error("runID empty")
	}
	if len(got) != 1 || got[0].Content != "hi" {
		t.Errorf("context: %v", got)
	}

	ev := <-sub
	if ev.Phase != PhaseStart {
		t.Errorf("phase: %s", ev.Phase)
	}
}

func TestConversationEngine_EndTurn(t *testing.T) {
	a := &mockAssembler{load: func(_, _ string) ([]llm.Message, error) { return nil, nil }}
	eng := NewConversationEngine(a)
	ctx := context.Background()

	runID, _, sub, _ := eng.StartTurn(ctx, "u1", "s1")
	eng.EndTurn(runID)

	<-sub // PhaseStart
	ev, ok := <-sub
	if !ok {
		t.Error("channel closed without PhaseEnd")
	}
	if ev.Phase != PhaseEnd {
		t.Errorf("phase: %s", ev.Phase)
	}
	_, ok = <-sub
	if ok {
		t.Error("channel should be closed")
	}
}
