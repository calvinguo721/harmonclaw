package gateway

import (
	"harmonclaw/api"
	"harmonclaw/skills"
)

// RegisterEdgeTTSSpeech registers POST /v1/audio/speech (OpenAI-compatible, audio/mpeg).
func (s *Server) RegisterEdgeTTSSpeech(tts *skills.EdgeTTSSkill) {
	if s == nil || s.Mux == nil || tts == nil {
		return
	}
	s.Mux.HandleFunc("POST /v1/audio/speech", api.NewEdgeTTSSpeechHandler(tts))
}
