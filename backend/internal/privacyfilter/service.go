package privacyfilter

import (
	"sync"
	"time"
)

// ScanRecord is a stored PrivacyScanRecord (§20) — a scan result plus identity.
type ScanRecord struct {
	ID          string     `json:"id"`
	SourceID    string     `json:"sourceId,omitempty"`
	OperationID string     `json:"operationId,omitempty"`
	Result      ScanResult `json:"result"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Service runs scans and stores their records for the Privacy API (§10.19).
type Service struct {
	mu      sync.Mutex
	records []ScanRecord
	seq     int
	now     func() time.Time
}

// NewService builds an empty privacy service.
func NewService() *Service { return &Service{now: time.Now} }

// DefaultService builds the default privacy service.
func DefaultService() *Service { return NewService() }

// Scan runs the deterministic scanner and stores the record.
func (s *Service) Scan(content, sourceID, operationID string, maxPreview int) ScanRecord {
	res := Scan(content, maxPreview)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	rec := ScanRecord{
		ID:          "scan-" + itoa(s.seq),
		SourceID:    sourceID,
		OperationID: operationID,
		Result:      res,
		CreatedAt:   s.now().UTC(),
	}
	s.records = append(s.records, rec)
	return rec
}

// Records returns a snapshot of all stored scans.
func (s *Service) Records() []ScanRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScanRecord, len(s.records))
	copy(out, s.records)
	return out
}

// Record returns a stored scan by id.
func (s *Service) Record(id string) (ScanRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if r.ID == id {
			return r, true
		}
	}
	return ScanRecord{}, false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
