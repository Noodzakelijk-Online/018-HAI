package pursuit

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/lifeops"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/resourceplanner"
	"github.com/google/uuid"
)

const portfolioCapacityFreshnessWindow = 24 * time.Hour

const (
	PortfolioCapacityNotEnforced   = "not_enforced"
	PortfolioCapacityApplied       = "applied"
	PortfolioCapacityMissing       = "missing"
	PortfolioCapacityStale         = "stale"
	PortfolioCapacityNeedsReview   = "needs_review"
	PortfolioCapacityUnavailable   = "unavailable"
	PortfolioCapacityOwnerMismatch = "owner_mismatch"
)

// PortfolioCapacityReader is intentionally read-only. Planning may consume a
// confirmed capacity snapshot, but it cannot create or alter personal state.
type PortfolioCapacityReader interface {
	LatestCapacity(ownerIdentity string) (*lifeops.CapacitySnapshot, error)
}

type PortfolioCapacityAssessment struct {
	Status               string     `json:"status"`
	SnapshotID           string     `json:"snapshotId,omitempty"`
	SnapshotStatus       string     `json:"snapshotStatus,omitempty"`
	CapturedAt           *time.Time `json:"capturedAt,omitempty"`
	FreshUntil           *time.Time `json:"freshUntil,omitempty"`
	Confidence           float64    `json:"confidence,omitempty"`
	TimeAvailableMinutes int        `json:"timeAvailableMinutes,omitempty"`
	AppliedMinutes       int        `json:"appliedMinutes,omitempty"`
	ConcurrentWorkLimit  int        `json:"concurrentWorkLimit,omitempty"`
	CurrentLoad          int        `json:"currentLoad,omitempty"`
	PlanningStepLimit    int        `json:"planningStepLimit,omitempty"`
	Constraints          []string   `json:"constraints,omitempty"`
	SourceLabel          string     `json:"sourceLabel,omitempty"`
	Reason               string     `json:"reason"`
}

// WithPortfolioCapacity binds portfolio decisions to the same owner-scoped
// capacity ledger shown in LifeOps. Non-canonical service implementations stay
// unchanged so protocol previews and narrow test doubles remain side-effect free.
func WithPortfolioCapacity(value Service, reader PortfolioCapacityReader) Service {
	concrete, ok := value.(*service)
	if !ok || concrete == nil || reader == nil {
		return value
	}
	concrete.portfolioCapacity = reader
	if concrete.portfolioCapacityNow == nil {
		concrete.portfolioCapacityNow = func() time.Time { return time.Now().UTC() }
	}
	return concrete
}

func withPortfolioCapacityClock(value Service, now func() time.Time) Service {
	concrete, ok := value.(*service)
	if !ok || concrete == nil || now == nil {
		return value
	}
	concrete.portfolioCapacityNow = now
	return concrete
}

func (s *service) applyPortfolioCapacity(
	ownerIdentity string,
	request PortfolioPlanningRequest,
	result *PortfolioPlanningResult,
	inputs map[uuid.UUID]models.Pursuit,
) (*lifeops.CapacitySnapshot, []resourceplanner.CapacityWindow, bool, error) {
	if s.portfolioCapacity == nil {
		availability, minutes := boundedPortfolioAvailability(request.Availability, 0)
		result.Capacity = &PortfolioCapacityAssessment{
			Status: PortfolioCapacityNotEnforced, AppliedMinutes: minutes,
			Reason: "no durable capacity reader is attached to this service",
		}
		return nil, availability, false, nil
	}

	snapshot, err := s.portfolioCapacity.LatestCapacity(ownerIdentity)
	if err != nil {
		if errors.Is(err, lifeops.ErrNotFound) {
			result.Status = "capacity_required"
			result.Capacity = &PortfolioCapacityAssessment{
				Status: PortfolioCapacityMissing,
				Reason: "record a current owner capacity check-in before portfolio planning",
			}
			appendPortfolioCapacityExclusions(result, inputs, "capacity_snapshot_required", result.Capacity.Reason)
			return nil, nil, true, nil
		}
		return nil, nil, true, fmt.Errorf("read owner capacity ledger: %w", err)
	}
	if snapshot == nil {
		return nil, nil, true, fmt.Errorf("read owner capacity ledger: empty snapshot")
	}

	now := time.Now().UTC()
	if s.portfolioCapacityNow != nil {
		now = s.portfolioCapacityNow().UTC()
	}
	capturedAt := snapshot.CapturedAt.UTC()
	freshUntil := capturedAt.Add(portfolioCapacityFreshnessWindow)
	assessment := &PortfolioCapacityAssessment{
		Status: PortfolioCapacityApplied, SnapshotID: snapshot.ID.String(),
		SnapshotStatus: snapshot.Status, CapturedAt: &capturedAt, FreshUntil: &freshUntil,
		Confidence: snapshot.Confidence, TimeAvailableMinutes: snapshot.TimeAvailableMinutes,
		ConcurrentWorkLimit: snapshot.ConcurrentWorkLimit, CurrentLoad: snapshot.CurrentLoad,
		PlanningStepLimit: snapshot.PlanningStepLimit,
		Constraints:       append([]string(nil), snapshot.Constraints...),
		SourceLabel:       strings.TrimSpace(snapshot.SourceLabel),
		Reason:            "fresh owner-confirmed capacity constrains schedule length and priority factors",
	}
	result.Capacity = assessment

	switch {
	case strings.TrimSpace(snapshot.OwnerIdentity) != ownerIdentity:
		assessment.Status = PortfolioCapacityOwnerMismatch
		assessment.Reason = "capacity snapshot owner does not match the authenticated pursuit owner"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_owner_mismatch", assessment.Reason)
		result.Status = "capacity_review_required"
		return nil, nil, true, nil
	case snapshot.ID == [16]byte{}:
		assessment.Status = PortfolioCapacityNeedsReview
		assessment.Reason = "capacity snapshot has no durable identifier"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_review_required", assessment.Reason)
		result.Status = "capacity_review_required"
		return nil, nil, true, nil
	case capturedAt.IsZero() || capturedAt.After(now.Add(time.Minute)) || !snapshot.Fresh || now.After(freshUntil):
		assessment.Status = PortfolioCapacityStale
		assessment.Reason = "the latest capacity snapshot is older than 24 hours or has an invalid timestamp"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_snapshot_stale", assessment.Reason)
		result.Status = "capacity_stale"
		return nil, nil, true, nil
	case snapshot.NeedsReview || snapshot.Confidence < 0.6 || snapshot.Status == lifeops.CapacityUnknown:
		assessment.Status = PortfolioCapacityNeedsReview
		assessment.Reason = "the latest capacity snapshot requires owner review or has insufficient confidence"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_review_required", assessment.Reason)
		result.Status = "capacity_review_required"
		return nil, nil, true, nil
	case snapshot.Status == lifeops.CapacityUnavailable || snapshot.TimeAvailableMinutes <= 0:
		assessment.Status = PortfolioCapacityUnavailable
		assessment.Reason = "the owner capacity snapshot does not permit schedulable work"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_unavailable", assessment.Reason)
		result.Status = "capacity_unavailable"
		return nil, nil, true, nil
	}

	availability, appliedMinutes := boundedPortfolioAvailability(request.Availability, snapshot.TimeAvailableMinutes)
	assessment.AppliedMinutes = appliedMinutes
	if appliedMinutes <= 0 {
		assessment.Status = PortfolioCapacityUnavailable
		assessment.Reason = "no owner availability remains after applying the confirmed capacity limit"
		appendPortfolioCapacityExclusions(result, inputs, "capacity_unavailable", assessment.Reason)
		result.Status = "capacity_unavailable"
		return nil, nil, true, nil
	}
	return snapshot, availability, false, nil
}

func boundedPortfolioAvailability(windows []PortfolioCapacityWindow, limitMinutes int) ([]resourceplanner.CapacityWindow, int) {
	type interval struct{ start, end time.Time }
	ordered := make([]interval, 0, len(windows))
	for _, window := range windows {
		ordered = append(ordered, interval{start: window.Start.UTC(), end: window.End.UTC()})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].start.Equal(ordered[j].start) {
			return ordered[i].end.Before(ordered[j].end)
		}
		return ordered[i].start.Before(ordered[j].start)
	})
	merged := make([]interval, 0, len(ordered))
	for _, item := range ordered {
		if len(merged) == 0 || item.start.After(merged[len(merged)-1].end) {
			merged = append(merged, item)
			continue
		}
		if item.end.After(merged[len(merged)-1].end) {
			merged[len(merged)-1].end = item.end
		}
	}

	remaining := limitMinutes
	unlimited := limitMinutes <= 0
	result := make([]resourceplanner.CapacityWindow, 0, len(merged))
	applied := 0
	for _, item := range merged {
		minutes := int(item.end.Sub(item.start) / time.Minute)
		if minutes <= 0 || (!unlimited && remaining <= 0) {
			continue
		}
		if !unlimited && minutes > remaining {
			minutes = remaining
		}
		end := item.start.Add(time.Duration(minutes) * time.Minute)
		result = append(result, resourceplanner.CapacityWindow{
			ResourceID: "owner-capacity", Start: item.start, End: end, CapacityUnits: 1,
		})
		applied += minutes
		if !unlimited {
			remaining -= minutes
		}
	}
	return result, applied
}

func appendPortfolioCapacityExclusions(result *PortfolioPlanningResult, inputs map[uuid.UUID]models.Pursuit, code, reason string) {
	ids := make([]uuid.UUID, 0, len(inputs))
	for id := range inputs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		result.Exclusions = append(result.Exclusions, portfolioExclusion(inputs[id], code, reason))
	}
}
