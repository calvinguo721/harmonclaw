package skills

import "encoding/json"

type SkillIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Core    string `json:"core"`
}

type SkillInput struct {
	TraceID   string            `json:"trace_id"`
	Text      string            `json:"text"`
	Args      map[string]string `json:"args"`
	LocalOnly bool              `json:"local_only"`
}

type SkillOutput struct {
	TraceID string          `json:"trace_id"`
	Status  string          `json:"status"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
	Metrics struct {
		Ms     int64 `json:"ms"`
		Bytes  int   `json:"bytes"`
		Tokens int   `json:"tokens"`
	} `json:"metrics"`
}

type Skill interface {
	GetIdentity() SkillIdentity
	Execute(input SkillInput) SkillOutput
}

var Registry = make(map[string]Skill)

func Register(s Skill) {
	id := s.GetIdentity()
	Registry[id.ID] = s
}
