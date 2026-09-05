package events

import (
	"sync"
	"time"
)

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

// Broker fans structured runtime events out to API clients. Slow clients drop
// intermediate progress updates instead of blocking inference or installation.
type Broker struct {
	mu          sync.Mutex
	next        uint64
	subscribers map[uint64]chan Event
}

func NewBroker() *Broker { return &Broker{subscribers: map[uint64]chan Event{}} }
func (b *Broker) Publish(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}
func (b *Broker) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	channel := make(chan Event, 64)
	b.subscribers[id] = channel
	b.mu.Unlock()
	return channel, func() {
		b.mu.Lock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}
}
