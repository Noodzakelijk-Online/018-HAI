package ambient

import (
	"automation-hub-backend/internal/models"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestOpportunityScorePenalizesRiskWithoutHidingUrgentSafetyWork(t *testing.T) {
	need := models.AmbientNeed{
		CurrentLevel:   35,
		TargetLevel:    90,
		PriorityWeight: 100,
	}
	item := models.AmbientOpportunity{
		Urgency:          100,
		Impact:           85,
		Effort:           35,
		Confidence:       85,
		Risk:             80,
		RequiresApproval: true,
	}
	if got := opportunityScore(item, need); got < 35 {
		t.Fatalf("urgent safety work score = %d, expected it to remain visible", got)
	}
}

func TestScanRejectsConcurrentRun(t *testing.T) {
	engine := NewService(&ambientRepositoryStub{}, nil, nil).(*service)
	engine.scanning.Store(true)

	if _, err := engine.Scan("manual"); !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("Scan error = %v, want ErrScanInProgress", err)
	}
}

func TestAcceptedOpportunityCannotBeDismissed(t *testing.T) {
	workflowID := uuid.New()
	item := &models.AmbientOpportunity{
		ID:         uuid.New(),
		WorkflowID: &workflowID,
		Status:     StatusAccepted,
	}
	engine := NewService(&ambientRepositoryStub{opportunity: item}, nil, nil)

	if _, err := engine.Dismiss(item.ID, ResolutionRequest{}); err == nil {
		t.Fatalf("expected accepted opportunity dismissal to be rejected")
	}
}

func TestAcceptProposedOpportunityStoresResolutionNote(t *testing.T) {
	workflowID := uuid.New()
	item := &models.AmbientOpportunity{
		ID:         uuid.New(),
		WorkflowID: &workflowID,
		Status:     StatusProposed,
	}
	repo := &ambientRepositoryStub{opportunity: item}
	engine := NewService(repo, nil, nil)

	accepted, err := engine.Accept(item.ID, ResolutionRequest{Note: "Proceed through the approval queue."})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if accepted.Status != StatusAccepted {
		t.Fatalf("status = %q, want accepted", accepted.Status)
	}
	if accepted.ResolutionNote == "" {
		t.Fatalf("expected operator resolution note to be retained")
	}
}

type ambientRepositoryStub struct {
	opportunity *models.AmbientOpportunity
}

func (r *ambientRepositoryStub) EnsureNeeds([]models.AmbientNeed) error {
	return nil
}

func (r *ambientRepositoryStub) Needs() ([]models.AmbientNeed, error) {
	return nil, nil
}

func (r *ambientRepositoryStub) UpdateNeed(need *models.AmbientNeed) (*models.AmbientNeed, error) {
	return need, nil
}

func (r *ambientRepositoryStub) FindOpportunity(uuid.UUID) (*models.AmbientOpportunity, error) {
	if r.opportunity == nil {
		return nil, errors.New("opportunity not found")
	}
	copy := *r.opportunity
	return &copy, nil
}

func (r *ambientRepositoryStub) FindOpportunityByFingerprint(string) (*models.AmbientOpportunity, error) {
	return nil, nil
}

func (r *ambientRepositoryStub) SaveOpportunity(item *models.AmbientOpportunity) (*models.AmbientOpportunity, error) {
	copy := *item
	r.opportunity = &copy
	return item, nil
}

func (r *ambientRepositoryStub) Opportunities(string, int) ([]models.AmbientOpportunity, error) {
	return nil, nil
}

func (r *ambientRepositoryStub) CreateScan(scan *models.AmbientScan) (*models.AmbientScan, error) {
	return scan, nil
}

func (r *ambientRepositoryStub) UpdateScan(scan *models.AmbientScan) (*models.AmbientScan, error) {
	return scan, nil
}

func (r *ambientRepositoryStub) Scans(int) ([]models.AmbientScan, error) {
	return nil, nil
}

func (r *ambientRepositoryStub) PruneScans(int) error {
	return nil
}
