// Package engine provides conversation and skill routing logic extracted from OpenClaw.
package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"harmonclaw/llm"
)

// TurnPhase represents lifecycle phase (OpenClaw-style).
const (
	PhaseStart   = "start"
	PhaseTool    = "tool"
	PhaseEnd     = "end"
	PhaseError   = "error"
)

// SessionScope defines how sessions are isolated (from OpenClaw session.dmScope).
const (
	ScopeMain                = "main"                 // all DMs share one session
	ScopePerChannelPeer      = "per-channel-peer"     // isolate by channel + sender
	ScopePerAccountChannelPeer = "per-account-channel-peer"
)

// TurnEvent is a lifecycle event emitted during a conversation turn.
type TurnEvent struct {
	Phase   string
	RunID   string
	Payload any
	At      time.Time
}

// ContextAssembler builds LLM context from history and memory.
type ContextAssembler interface {
	LoadContext(userID, sessionID string) ([]llm.Message, error)
	Append(userID, sessionID, role, content string) ([]llm.Message, error)
}

// ConversationEngine orchestrates turns with OpenClaw-style lifecycle.
type ConversationEngine struct {
	assemble ContextAssembler
	mu       sync.Mutex
	runs     map[string]chan TurnEvent
}

// NewConversationEngine creates an engine backed by the given assembler (e.g. Butler MemoryManager).
func NewConversationEngine(assemble ContextAssembler) *ConversationEngine {
	return &ConversationEngine{
		assemble: assemble,
		runs:    make(map[string]chan TurnEvent),
	}
}

// ResolveSessionKey maps (agentID, channel, peer, scope) to session key (OpenClaw-style).
func ResolveSessionKey(agentID, channel, peerID, scope string) string {
	if agentID == "" {
		agentID = "main"
	}
	switch scope {
	case ScopePerChannelPeer:
		return fmt.Sprintf("agent:%s:%s:dm:%s", agentID, channel, peerID)
	case ScopePerAccountChannelPeer:
		return fmt.Sprintf("agent:%s:default:%s:dm:%s", agentID, channel, peerID)
	default:
		return fmt.Sprintf("agent:%s:main", agentID)
	}
}

// StartTurn begins a turn, returns runID and context. Emits PhaseStart.
func (e *ConversationEngine) StartTurn(ctx context.Context, userID, sessionID string) (runID string, messages []llm.Message, sub <-chan TurnEvent, err error) {
	messages, err = e.assemble.LoadContext(userID, sessionID)
	if err != nil {
		return "", nil, nil, err
	}
	runID = fmt.Sprintf("%d", time.Now().UnixNano())
	ch := make(chan TurnEvent, 16)
	e.mu.Lock()
	e.runs[runID] = ch
	e.mu.Unlock()

	ch <- TurnEvent{Phase: PhaseStart, RunID: runID, Payload: nil, At: time.Now()}
	return runID, messages, ch, nil
}

// AppendUserMessage appends user message and returns updated context.
func (e *ConversationEngine) AppendUserMessage(userID, sessionID, content string) ([]llm.Message, error) {
	return e.assemble.Append(userID, sessionID, "user", content)
}

// AppendAssistantMessage appends assistant reply.
func (e *ConversationEngine) AppendAssistantMessage(userID, sessionID, content string) ([]llm.Message, error) {
	return e.assemble.Append(userID, sessionID, "assistant", content)
}

// EmitTool emits a tool event for the run.
func (e *ConversationEngine) EmitTool(runID string, payload any) {
	e.mu.Lock()
	ch := e.runs[runID]
	e.mu.Unlock()
	if ch != nil {
		select {
		case ch <- TurnEvent{Phase: PhaseTool, RunID: runID, Payload: payload, At: time.Now()}:
		default:
		}
	}
}

// EndTurn marks turn complete and closes the event channel.
func (e *ConversationEngine) EndTurn(runID string) {
	e.finishRun(runID, PhaseEnd, nil)
}

// ErrorTurn marks turn failed.
func (e *ConversationEngine) ErrorTurn(runID string, err error) {
	e.finishRun(runID, PhaseError, err)
}

func (e *ConversationEngine) finishRun(runID, phase string, errVal error) {
	e.mu.Lock()
	ch := e.runs[runID]
	delete(e.runs, runID)
	e.mu.Unlock()
	if ch != nil {
		payload := any(nil)
		if errVal != nil {
			payload = errVal.Error()
		}
		ch <- TurnEvent{Phase: phase, RunID: runID, Payload: payload, At: time.Now()}
		close(ch)
	}
}
