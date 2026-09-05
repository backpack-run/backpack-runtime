package events

import (
	"testing"
	"time"
)

func TestBrokerPublishesStructuredEvents(t *testing.T) {
	broker := NewBroker()
	channel, unsubscribe := broker.Subscribe()
	defer unsubscribe()
	broker.Publish(Event{Type: ModelDownloadProgress, Kind: Progress, Current: 5, Total: 10})
	select {
	case event := <-channel:
		if event.Type != ModelDownloadProgress || event.Current != 5 || event.At.IsZero() {
			t.Fatalf("event %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not delivered")
	}
}
