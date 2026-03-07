package engine

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testConfigPath() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "testdata", "intents.json")
}

func TestIntentRecognizer_Recognize_Chat(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	tests := []struct {
		text string
		want string
	}{
		{"你好", IntentChat},
		{"hello", IntentChat},
		{"hi", IntentChat},
		{"hey there", IntentChat},
		{"在吗", IntentChat},
		{"聊聊吧", IntentChat},
		{"how are you", IntentChat},
		{"今天天气不错", IntentChat},
	}
	for _, tt := range tests {
		got := r.Recognize(tt.text, "s1")
		if got.Intent != tt.want {
			t.Errorf("Recognize(%q) intent=%s, want %s", tt.text, got.Intent, tt.want)
		}
	}
}

func TestIntentRecognizer_Recognize_Search(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	tests := []struct {
		text string
		want string
	}{
		{"搜索天气", IntentSearch},
		{"查找一下", IntentSearch},
		{"search for go", IntentSearch},
		{"查一下北京", IntentSearch},
		{"帮我找资料", IntentSearch},
		{"搜一下", IntentSearch},
	}
	for _, tt := range tests {
		got := r.Recognize(tt.text, "s1")
		if got.Intent != tt.want {
			t.Errorf("Recognize(%q) intent=%s, want %s", tt.text, got.Intent, tt.want)
		}
	}
}

func TestIntentRecognizer_Recognize_Skill(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	tests := []struct {
		text string
		want string
	}{
		{"执行 web_search", IntentSkill},
		{"运行 doc_perceiver", IntentSkill},
		{"调用技能", IntentSkill},
	}
	for _, tt := range tests {
		got := r.Recognize(tt.text, "s1")
		if got.Intent != tt.want {
			t.Errorf("Recognize(%q) intent=%s, want %s", tt.text, got.Intent, tt.want)
		}
	}
}

func TestIntentRecognizer_Recognize_Memory(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	tests := []struct {
		text string
		want string
	}{
		{"你还记得吗", IntentMemory},
		{"之前说过什么", IntentMemory},
		{"recall last", IntentMemory},
		{"remember that", IntentMemory},
	}
	for _, tt := range tests {
		got := r.Recognize(tt.text, "s1")
		if got.Intent != tt.want {
			t.Errorf("Recognize(%q) intent=%s, want %s", tt.text, got.Intent, tt.want)
		}
	}
}

func TestIntentRecognizer_Recognize_System(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	tests := []struct {
		text string
		want string
	}{
		{"状态", IntentSystem},
		{"健康", IntentSystem},
		{"version", IntentSystem},
		{"help", IntentSystem},
	}
	for _, tt := range tests {
		got := r.Recognize(tt.text, "s1")
		if got.Intent != tt.want {
			t.Errorf("Recognize(%q) intent=%s, want %s", tt.text, got.Intent, tt.want)
		}
	}
}

func TestIntentRecognizer_Recognize_MultiTurn(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())

	r.Recognize("搜索北京天气", "s2")
	got := r.Recognize("继续", "s2")
	if got.Intent != IntentSearch {
		t.Errorf("multi-turn 继续: intent=%s, want search", got.Intent)
	}
}

func TestIntentRecognizer_Recognize_Empty(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())
	got := r.Recognize("", "s1")
	if got.Intent != IntentChat || got.Confidence != 0 {
		t.Errorf("empty: intent=%s conf=%f", got.Intent, got.Confidence)
	}
}

func TestIntentRecognizer_Confidence(t *testing.T) {
	r := NewIntentRecognizer(testConfigPath())
	got := r.Recognize("搜索", "s1")
	if got.Confidence <= 0 || got.Confidence > 1 {
		t.Errorf("confidence out of range: %f", got.Confidence)
	}
}

func TestIntentRecognizer_DefaultRules(t *testing.T) {
	r := NewIntentRecognizer("/nonexistent/path.json")
	got := r.Recognize("hello", "s1")
	if got.Intent != IntentChat {
		t.Errorf("default rules: intent=%s", got.Intent)
	}
}
