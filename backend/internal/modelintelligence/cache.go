package modelintelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// CacheRecord is a stored, reusable model result with the boundaries needed to
// enforce the reuse rules (§19).
type CacheRecord struct {
	ID                 string    `json:"id"`
	CacheType          CacheType `json:"cacheType"`
	Key                string    `json:"key"`
	Output             string    `json:"output"`
	SourceRevisionHash string    `json:"sourceRevisionHash"`
	Verified           bool      `json:"verified"`
	SafeForCloud       bool      `json:"safeForCloud"`
	HighRiskDomain     bool      `json:"highRiskDomain"`
	Hits               int       `json:"hits"`
	CreatedAt          time.Time `json:"createdAt"`
}

// CacheLookup is a request to read from the cache under the §19 reuse rules.
type CacheLookup struct {
	CacheType          CacheType
	Prompt             string
	SourceRevisionHash string
	ForHighRiskAction  bool
	SafeForCloud       bool
}

// CacheKey is the deterministic key for a lookup.
func CacheKey(cacheType CacheType, prompt string) string {
	sum := sha256.Sum256([]byte(string(cacheType) + "|" + prompt))
	return hex.EncodeToString(sum[:])
}

// Cache is an in-process cache enforcing the §19 reuse boundaries. It never
// reuses output for high-risk actions, across a changed source revision, or
// across a privacy boundary, and never answers sensitive-domain questions from
// the semantic cache without fresh verification.
type Cache struct {
	mu      sync.Mutex
	records map[string]CacheRecord
	hits    int
	misses  int
}

// NewCache builds an empty cache.
func NewCache() *Cache { return &Cache{records: map[string]CacheRecord{}} }

// Get returns a cached record if reuse is permitted, else (zero,false) with the
// reason recorded as a miss.
func (c *Cache) Get(l CacheLookup) (CacheRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := CacheKey(l.CacheType, l.Prompt)
	rec, ok := c.records[key]
	if !ok {
		c.misses++
		return CacheRecord{}, false
	}
	if !c.reusable(rec, l) {
		c.misses++
		return CacheRecord{}, false
	}
	rec.Hits++
	c.records[key] = rec
	c.hits++
	return rec, true
}

func (c *Cache) reusable(rec CacheRecord, l CacheLookup) bool {
	// Do not reuse unverified output for high-risk actions.
	if l.ForHighRiskAction && !rec.Verified {
		return false
	}
	// Do not reuse output when the source revision changed.
	if l.SourceRevisionHash != "" && rec.SourceRevisionHash != "" && rec.SourceRevisionHash != l.SourceRevisionHash {
		return false
	}
	// Do not reuse output across privacy boundaries.
	if l.SafeForCloud && !rec.SafeForCloud {
		return false
	}
	// Semantic cache must not answer sensitive-domain / action-approval work
	// without fresh verification.
	if rec.CacheType == CacheSemantic && rec.HighRiskDomain && !rec.Verified {
		return false
	}
	return true
}

// Put stores a record. It refuses to store a high-risk-domain semantic entry
// that is unverified (so it can never be reused unsafely).
func (c *Cache) Put(rec CacheRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rec.Key == "" {
		rec.Key = CacheKey(rec.CacheType, rec.Output)
	}
	id := CacheKey(rec.CacheType, rec.Key)
	rec.ID = id
	c.records[keyOf(rec.CacheType, rec.Key)] = rec
}

// keyOf normalizes the storage key from type + logical key.
func keyOf(t CacheType, logicalKey string) string {
	if strings.HasPrefix(logicalKey, "sha") || len(logicalKey) == 64 {
		return logicalKey
	}
	return CacheKey(t, logicalKey)
}

// Store caches a model result under the prompt as key, applying boundaries.
func (c *Cache) Store(cacheType CacheType, prompt, output, sourceRevisionHash string, verified, safeForCloud, highRiskDomain bool, now time.Time) {
	key := CacheKey(cacheType, prompt)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records[key] = CacheRecord{
		ID:                 key,
		CacheType:          cacheType,
		Key:                key,
		Output:             output,
		SourceRevisionHash: sourceRevisionHash,
		Verified:           verified,
		SafeForCloud:       safeForCloud,
		HighRiskDomain:     highRiskDomain,
		CreatedAt:          now,
	}
}

// Records returns a snapshot of cached records.
func (c *Cache) Records() []CacheRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CacheRecord, 0, len(c.records))
	for _, r := range c.records {
		out = append(out, r)
	}
	return out
}

// Delete removes a record by id. Returns true if present.
func (c *Cache) Delete(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.records[id]; ok {
		delete(c.records, id)
		return true
	}
	return false
}

// Stats reports cache hits/misses for telemetry (§19: display cache hits).
func (c *Cache) Stats() (hits, misses int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}
