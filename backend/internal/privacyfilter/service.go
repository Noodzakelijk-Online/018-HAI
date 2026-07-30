package privacyfilter

import (
	"strings"
	"sync"
	"time"
)

// ScanRecord is a stored PrivacyScanRecord (§20) — a scan result plus identity.
type ScanRecord struct {
	ID            string     `json:"id"`
	OwnerIdentity string     `json:"-"`
	SourceID      string     `json:"sourceId,omitempty"`
	OperationID   string     `json:"operationId,omitempty"`
	Result        ScanResult `json:"result"`
	CreatedAt     time.Time  `json:"createdAt"`
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
	return s.scanForOwner("", content, sourceID, operationID, maxPreview)
}

// ScanForOwner stores a scan under a verified owner identity.
func (s *Service) ScanForOwner(ownerIdentity, content, sourceID, operationID string, maxPreview int) ScanRecord {
	return s.scanForOwner(ownerIdentity, content, sourceID, operationID, maxPreview)
}

func (s *Service) scanForOwner(ownerIdentity, content, sourceID, operationID string, maxPreview int) ScanRecord {
	res := Scan(content, maxPreview)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	rec := ScanRecord{
		ID:            "scan-" + itoa(s.seq),
		OwnerIdentity: strings.TrimSpace(ownerIdentity),
		SourceID:      sourceID,
		OperationID:   operationID,
		Result:        res,
		CreatedAt:     s.now().UTC(),
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

// RecordsForOwner returns only records belonging to the verified owner.
// Ownerless system records are quarantined from authenticated history views.
func (s *Service) RecordsForOwner(ownerIdentity string) []ScanRecord {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScanRecord, 0, len(s.records))
	for _, record := range s.records {
		if strings.TrimSpace(record.OwnerIdentity) == ownerIdentity {
			out = append(out, record)
		}
	}
	return out
}

// RecordForOwner returns a scan only when both the record ID and owner match.
func (s *Service) RecordForOwner(ownerIdentity, id string) (ScanRecord, bool) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return ScanRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && strings.TrimSpace(record.OwnerIdentity) == ownerIdentity {
			return record, true
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
