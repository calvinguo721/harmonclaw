// Package bus provides inter-core message passing for Governor, Butler, Architect.
package bus

const bufSize = 64

var ch = make(chan Message, bufSize)

type CoreID string

const (
	Governor  CoreID = "governor"
	Butler    CoreID = "butler"
	Architect CoreID = "architect"
)

type Message struct {
	From    CoreID
	To      CoreID
	Type    string
	Payload any
	TraceID string
}

func Send(m Message) {
	select {
	case ch <- m:
	default:
		// channel full, drop (caller may log)
	}
}

func Subscribe() <-chan Message {
	return ch
}
