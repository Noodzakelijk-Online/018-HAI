package accountfeed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/privacyfilter"

	"github.com/google/uuid"
)

// AuditEvent is an immutable audit row for a feed (§10.19 feed audit).
type AuditEvent struct {
	ID        string    `json:"id"`
	FeedID    string    `json:"feedId"`
	EventType string    `json:"eventType"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// SyncReport summarizes one feed sync.
type SyncReport struct {
	FeedID            string   `json:"feedId"`
	ItemsRead         int      `json:"itemsRead"`
	OperationsCreated int      `json:"operationsCreated"`
	OperationsRefresh int      `json:"operationsRefreshed"`
	PrivacyFlagged    int      `json:"privacyFlagged"`
	Cursor            string   `json:"cursor,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

// FeedHealth is a feed's truthful health for the dashboard.
type FeedHealth struct {
	Feed             Feed             `json:"feed"`
	ConnectionStatus ConnectionStatus `json:"connectionStatus"`
	LastSyncedAt     *time.Time       `json:"lastSyncedAt,omitempty"`
	LastItemsRead    int              `json:"lastItemsRead"`
}

// Registry is the account feed registry (§14). It stores feed configs, syncs
// them (fetch → privacy scan → dedupe-ingest into the Operation Ledger), and
// records an audit trail. It never fakes provider access.
type Registry struct {
	mu      sync.Mutex
	feeds   map[uuid.UUID]Feed
	audits  map[uuid.UUID][]AuditEvent
	lastRun map[uuid.UUID]*time.Time
	lastN   map[uuid.UUID]int
	seq     int

	ops     *operations.Service
	privacy *privacyfilter.Service
	opts    FetchOptions
	maxC    int
	maxM    int
	now     func() time.Time
}

// NewRegistry builds a feed registry.
func NewRegistry(ops *operations.Service, privacy *privacyfilter.Service, opts FetchOptions) *Registry {
	return &Registry{
		feeds:   map[uuid.UUID]Feed{},
		audits:  map[uuid.UUID][]AuditEvent{},
		lastRun: map[uuid.UUID]*time.Time{},
		lastN:   map[uuid.UUID]int{},
		ops:     ops,
		privacy: privacy,
		opts:    opts,
		maxC:    defaultMaxContentBytes,
		maxM:    defaultMaxMetadataBytes,
		now:     time.Now,
	}
}

// Register validates and stores a feed, returning it with an assigned id.
func (r *Registry) Register(feed Feed) (Feed, error) {
	if feed.WorkspaceID == "" {
		feed.WorkspaceID = "local"
	}
	if err := feed.Validate(); err != nil {
		return Feed{}, err
	}
	// The registry requires a valid, supported provider contract (§14).
	if _, err := ParseProvider(feed.Provider); err != nil {
		return Feed{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if feed.ID == uuid.Nil {
		feed.ID = uuid.New()
	}
	r.feeds[feed.ID] = feed
	r.appendAudit(feed.ID, "registered", "feed registered: "+feed.Name)
	return feed, nil
}

// List returns all registered feeds.
func (r *Registry) List() []Feed {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Feed, 0, len(r.feeds))
	for _, f := range r.feeds {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a feed by id.
func (r *Registry) Get(id uuid.UUID) (Feed, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.feeds[id]
	return f, ok
}

// Patch updates mutable feed fields (enabled, name, operationType).
func (r *Registry) Patch(id uuid.UUID, enabled *bool, name, operationType *string) (Feed, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.feeds[id]
	if !ok {
		return Feed{}, false
	}
	if enabled != nil {
		f.Enabled = *enabled
	}
	if name != nil && *name != "" {
		f.Name = *name
	}
	if operationType != nil {
		f.OperationType = *operationType
	}
	r.feeds[id] = f
	r.appendAudit(id, "updated", "feed updated")
	return f, true
}

// Health returns truthful health for every feed.
func (r *Registry) Health() []FeedHealth {
	return r.healthFor(func(Feed) bool { return true })
}

// HealthForOwner returns health only for feeds owned by owner. The account-feed
// registry is in-memory today, so ownership must be applied while holding the
// same lock that reads feed state rather than after a broad list is returned.
func (r *Registry) HealthForOwner(owner string) []FeedHealth {
	return r.healthFor(func(feed Feed) bool { return feed.OwnerUserID == owner })
}

func (r *Registry) healthFor(visible func(Feed) bool) []FeedHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]FeedHealth, 0, len(r.feeds))
	for id, f := range r.feeds {
		if !visible(f) {
			continue
		}
		status := ConnAvailable
		if p, err := ParseProvider(f.Provider); err == nil {
			if b, ok := Bridge(p); ok {
				status = b.ConnectionStatus()
			}
		}
		out = append(out, FeedHealth{Feed: f, ConnectionStatus: status, LastSyncedAt: r.lastRun[id], LastItemsRead: r.lastN[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Feed.Name < out[j].Feed.Name })
	return out
}

// GetForOwner looks up a feed without revealing whether another owner has the
// requested identifier.
func (r *Registry) GetForOwner(id uuid.UUID, owner string) (Feed, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	feed, ok := r.feeds[id]
	return feed, ok && feed.OwnerUserID == owner
}

// PatchForOwner updates only a feed owned by owner.
func (r *Registry) PatchForOwner(id uuid.UUID, owner string, enabled *bool, name, operationType *string) (Feed, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	feed, ok := r.feeds[id]
	if !ok || feed.OwnerUserID != owner {
		return Feed{}, false
	}
	if enabled != nil {
		feed.Enabled = *enabled
	}
	if name != nil && *name != "" {
		feed.Name = *name
	}
	if operationType != nil {
		feed.OperationType = *operationType
	}
	r.feeds[id] = feed
	r.appendAudit(id, "updated", "feed updated")
	return feed, true
}

// Sync fetches a feed, privacy-scans each item, and ingests operations.
func (r *Registry) Sync(ctx context.Context, id uuid.UUID) (SyncReport, bool) {
	r.mu.Lock()
	feed, ok := r.feeds[id]
	r.mu.Unlock()
	if !ok {
		return SyncReport{}, false
	}
	return r.syncFeed(ctx, feed), true
}

// SyncForOwner runs only a feed owned by owner. A foreign identifier is
// indistinguishable from a missing one to callers.
func (r *Registry) SyncForOwner(ctx context.Context, id uuid.UUID, owner string) (SyncReport, bool) {
	r.mu.Lock()
	feed, ok := r.feeds[id]
	r.mu.Unlock()
	if !ok || feed.OwnerUserID != owner {
		return SyncReport{}, false
	}
	return r.syncFeed(ctx, feed), true
}

// SyncDue syncs all enabled feeds.
func (r *Registry) SyncDue(ctx context.Context) []SyncReport {
	return r.syncDueFor(ctx, func(Feed) bool { return true })
}

// SyncDueForOwner runs enabled feeds only for one authenticated owner.
func (r *Registry) SyncDueForOwner(ctx context.Context, owner string) []SyncReport {
	return r.syncDueFor(ctx, func(feed Feed) bool { return feed.OwnerUserID == owner })
}

func (r *Registry) syncDueFor(ctx context.Context, visible func(Feed) bool) []SyncReport {
	r.mu.Lock()
	var due []Feed
	for _, f := range r.feeds {
		if f.Enabled && visible(f) {
			due = append(due, f)
		}
	}
	r.mu.Unlock()
	reports := make([]SyncReport, 0, len(due))
	for _, f := range due {
		reports = append(reports, r.syncFeed(ctx, f))
	}
	return reports
}

func (r *Registry) syncFeed(ctx context.Context, feed Feed) SyncReport {
	rep := SyncReport{FeedID: feed.ID.String()}
	data, err := fetchFeedBytes(ctx, feed, r.opts)
	if err != nil {
		message := publicSyncError(err)
		rep.Errors = append(rep.Errors, message)
		r.recordSync(feed.ID, 0, "sync_failed", message)
		return rep
	}
	parsed, err := ParseGenericFeed(data, r.maxC, r.maxM)
	if err != nil {
		message := publicSyncError(err)
		rep.Errors = append(rep.Errors, message)
		r.recordSync(feed.ID, 0, "sync_failed", message)
		return rep
	}
	rep.Cursor = parsed.Cursor
	for _, item := range parsed.Items {
		rep.ItemsRead++
		// Privacy filter runs before storage/model use (§14 Fetcher -> Privacy).
		if r.privacy != nil {
			scan := r.privacy.Scan(item.Content, item.ExternalID, "", 280)
			if !scan.Result.SafeForCloudModel || scan.Result.PrivacyRiskLevel == privacyfilter.RiskHigh || scan.Result.PrivacyRiskLevel == privacyfilter.RiskCritical {
				rep.PrivacyFlagged++
			}
		}
		in, err := feed.ToOperationInput(item.ToFeedItem())
		if err != nil {
			rep.Errors = append(rep.Errors, "a feed item was rejected during bounded validation")
			continue
		}
		res, err := r.ops.Ingest(in)
		if err != nil {
			rep.Errors = append(rep.Errors, "a feed item could not be recorded")
			continue
		}
		if res.Created {
			rep.OperationsCreated++
		} else {
			rep.OperationsRefresh++
		}
	}
	r.recordSync(feed.ID, rep.ItemsRead, "synced", fmt.Sprintf("read %d items, %d new operations, %d privacy-flagged", rep.ItemsRead, rep.OperationsCreated, rep.PrivacyFlagged))
	return rep
}

// publicSyncError keeps browser and audit responses useful without exposing
// local filesystem locations, upstream URLs, parser payloads, or transport
// internals. Detailed runner diagnostics stay at the local operator boundary.
func publicSyncError(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "http feeds are disabled"):
		return "HTTP feed sync is disabled by policy"
	case strings.Contains(message, "unsafe feed path"):
		return "feed path is not approved"
	case strings.Contains(message, "exceeds"):
		return "feed content exceeds the configured size limit"
	case strings.Contains(message, "must be a json") || strings.Contains(message, "invalid character"):
		return "feed content is not valid JSON"
	default:
		return "feed sync failed; inspect local operator diagnostics"
	}
}

// Audit returns a feed's audit trail (newest first).
func (r *Registry) Audit(id uuid.UUID) []AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := r.audits[id]
	out := make([]AuditEvent, len(events))
	for i := range events {
		out[i] = events[len(events)-1-i]
	}
	return out
}

// AuditForOwner returns an immutable audit view only when the feed belongs to
// owner. It deliberately uses the same not-found result for foreign feeds.
func (r *Registry) AuditForOwner(id uuid.UUID, owner string) ([]AuditEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	feed, ok := r.feeds[id]
	if !ok || feed.OwnerUserID != owner {
		return nil, false
	}
	events := r.audits[id]
	out := make([]AuditEvent, len(events))
	for i := range events {
		out[i] = events[len(events)-1-i]
	}
	return out, true
}

func (r *Registry) recordSync(id uuid.UUID, items int, eventType, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UTC()
	r.lastRun[id] = &now
	r.lastN[id] = items
	r.appendAudit(id, eventType, msg)
}

// appendAudit must be called with r.mu held.
func (r *Registry) appendAudit(id uuid.UUID, eventType, msg string) {
	r.seq++
	r.audits[id] = append(r.audits[id], AuditEvent{
		ID:        fmt.Sprintf("afa-%d", r.seq),
		FeedID:    id.String(),
		EventType: eventType,
		Message:   msg,
		CreatedAt: r.now().UTC(),
	})
}
