package events

import "time"

type Kind string

const (
	Status   Kind = "status"
	Progress Kind = "progress"
	Warning  Kind = "warning"
	Token    Kind = "token"
	Complete Kind = "complete"
)

type Event struct {
	Kind       Kind      `json:"kind"`
	Message    string    `json:"message,omitempty"`
	Current    int64     `json:"current,omitempty"`
	Total      int64     `json:"total,omitempty"`
	Percentage float64   `json:"percentage,omitempty"`
	At         time.Time `json:"at"`
}

type Sink func(Event)

func Emit(sink Sink, event Event) {
	if sink != nil {
		event.At = time.Now().UTC()
		sink(event)
	}
}
