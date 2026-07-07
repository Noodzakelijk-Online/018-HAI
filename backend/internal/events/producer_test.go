package events

import "testing"

// When Kafka is disabled/unavailable, DefaultPublisher returns a Publisher with
// a nil producer; publishing and closing it must be safe no-ops so the rest of
// the app runs normally without Kafka.
func TestNoopPublisherDoesNotPanicOrError(t *testing.T) {
	p := &Publisher{producer: nil, topic: "automation-events"}
	if err := p.Publish(&AutomationEvent{}); err != nil {
		t.Fatalf("no-op publish should not error: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("no-op close should not error: %v", err)
	}
}

func TestNonEmptyBrokersFiltersBlanks(t *testing.T) {
	got := nonEmptyBrokers([]string{"", "   ", "kafka1:9092", ""})
	if len(got) != 1 || got[0] != "kafka1:9092" {
		t.Fatalf("nonEmptyBrokers = %v, want [kafka1:9092]", got)
	}
}
