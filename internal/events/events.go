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
	Type       string    `json:"type,omitempty"`
	Kind       Kind      `json:"kind"`
	Message    string    `json:"message,omitempty"`
	Current    int64     `json:"current,omitempty"`
	Total      int64     `json:"total,omitempty"`
	Percentage float64   `json:"percentage,omitempty"`
	At         time.Time `json:"at"`
}

const (
	ModelResolve            = "model.resolve"
	ModelDownloadStarted    = "model.download.started"
	ModelDownloadProgress   = "model.download.progress"
	ModelVerifyComplete     = "model.verify.complete"
	RuntimePreparing        = "runtime.prepare"
	RuntimeDownloadStarted  = "runtime.download.started"
	RuntimeDownloadProgress = "runtime.download.progress"
	RuntimeInstalled        = "runtime.installed"
	RuntimeStarting         = "runtime.starting"
	RuntimeReady            = "runtime.ready"
	GenerationToken         = "generation.token"
	GenerationComplete      = "generation.complete"
	RuntimeError            = "runtime.error"
	ModelSyncProgress       = "model.sync.progress"
)

type Sink func(Event)

func Emit(sink Sink, event Event) {
	if sink != nil {
		event.At = time.Now().UTC()
		sink(event)
	}
}
