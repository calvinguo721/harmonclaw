package sandbox

var approvedSkills = map[string]bool{
	"skill_healthcheck": true,
}

type Guard interface {
	CheckSkill(skillID string) (allowed bool, verdict string)
}

type Whitelist struct{}

func NewWhitelist() *Whitelist {
	return &Whitelist{}
}

func (wl *Whitelist) CheckSkill(skillID string) (bool, string) {
	if approvedSkills[skillID] {
		return true, "APPROVED"
	}
	return false, "BLOCKED: Unauthorized skill access"
}
