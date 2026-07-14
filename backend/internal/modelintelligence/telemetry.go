package modelintelligence

import (
	"sync"
	"time"
)

// ModelRunTelemetry is one observed model call (§10.9 telemetry transaction).
// It is only ever created from a real call — never fabricated.
type ModelRunTelemetry struct {
	ID              string      `json:"id"`
	ProviderID      string      `json:"providerId"`
	ModelID         string      `json:"modelId"`
	Lane            RoutingLane `json:"lane"`
	OperationID     string      `json:"operationId,omitempty"`
	InputTokens     int         `json:"inputTokens"`
	OutputTokens    int         `json:"outputTokens"`
	DurationMs      int64       `json:"durationMs"`
	TokensPerSecond float64     `json:"tokensPerSecond"`
	OK              bool        `json:"ok"`
	CacheHit        bool        `json:"cacheHit"`
	CreatedAt       time.Time   `json:"createdAt"`
}

// TelemetryStore is an in-process store of model-run telemetry. It fronts an
// optional durable repository: rows are persisted on Record and seeded on start,
// so telemetry survives restart while queries stay in-memory-fast.
type TelemetryStore struct {
	mu      sync.Mutex
	records []ModelRunTelemetry
	seq     int
	persist func(ModelRunTelemetry) // optional durable sink
}

// NewTelemetryStore builds an empty store.
func NewTelemetryStore() *TelemetryStore { return &TelemetryStore{} }

// SetPersist installs a durable sink called for every recorded row.
func (s *TelemetryStore) SetPersist(fn func(ModelRunTelemetry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persist = fn
}

// Seed loads pre-existing (durable) telemetry into the store on startup.
func (s *TelemetryStore) Seed(rows []ModelRunTelemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rows...)
	s.seq += len(rows)
}

// Record appends a telemetry row (persisting it if a sink is set) and returns it
// with an assigned id.
func (s *TelemetryStore) Record(t ModelRunTelemetry) ModelRunTelemetry {
	s.mu.Lock()
	s.seq++
	t.ID = "mrt-" + itoa(s.seq)
	s.records = append(s.records, t)
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		persist(t)
	}
	return t
}

// All returns a snapshot of all telemetry.
func (s *TelemetryStore) All() []ModelRunTelemetry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ModelRunTelemetry, len(s.records))
	copy(out, s.records)
	return out
}

// LaneWinner is the best observed model for a lane by tokens/sec.
type LaneWinner struct {
	Lane            RoutingLane `json:"lane"`
	ProviderID      string      `json:"providerId"`
	ModelID         string      `json:"modelId"`
	TokensPerSecond float64     `json:"tokensPerSecond"`
	Runs            int         `json:"runs"`
}

// LaneWinners computes the fastest observed model per lane from telemetry only
// (§dashboard: model lane winners). Lanes with no observed successful run are
// omitted — no winner is invented.
func (s *TelemetryStore) LaneWinners() []LaneWinner {
	s.mu.Lock()
	defer s.mu.Unlock()
	type agg struct {
		bestTPS float64
		prov    string
		model   string
		runs    int
	}
	byLane := map[RoutingLane]*agg{}
	for _, r := range s.records {
		if !r.OK {
			continue
		}
		a := byLane[r.Lane]
		if a == nil {
			a = &agg{}
			byLane[r.Lane] = a
		}
		a.runs++
		if r.TokensPerSecond > a.bestTPS {
			a.bestTPS = r.TokensPerSecond
			a.prov = r.ProviderID
			a.model = r.ModelID
		}
	}
	out := make([]LaneWinner, 0, len(byLane))
	for _, lane := range allLanes() {
		if a, ok := byLane[lane]; ok {
			out = append(out, LaneWinner{Lane: lane, ProviderID: a.prov, ModelID: a.model, TokensPerSecond: a.bestTPS, Runs: a.runs})
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
