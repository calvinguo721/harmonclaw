package sandbox

type Rule struct {
	Name    string
	Pattern string
}

type Result struct {
	Allowed bool
	Reason  string
}

type Guard interface {
	Check(input string) Result
}
