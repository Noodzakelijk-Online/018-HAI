package operations

import (
	"errors"
	"strings"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ErrNotFound is returned when an operation does not exist.
var ErrNotFound = errors.New("operations: not found")

// ErrDuplicateDedupeKey means another active operation already represents the
// same owner-scoped source revision. Callers must load that operation instead
// of treating the collision as a failed ingestion.
var ErrDuplicateDedupeKey = errors.New("operations: duplicate active dedupe key")

// Filter narrows a List query.
type Filter struct {
	OwnerUserID string
	WorkspaceID string
	Status      OperationStatus // optional
	RiskLevel   RiskLevel       // optional
	Limit       int
	Offset      int
}

// Dashboard is the Background Operations dashboard roll-up (§24).
type Dashboard struct {
	CountsByStatus map[string]int     `json:"countsByStatus"`
	CountsByRisk   map[string]int     `json:"countsByRisk"`
	NeedsRobert    int                `json:"needsRobert"`
	DoneWhileAway  int                `json:"doneWhileAway"`
	Blocked        int                `json:"blocked"`
	Running        int                `json:"running"`
	Failed         int                `json:"failed"`
	Recent         []models.Operation `json:"recent"`
}

// Repository is the Operation Ledger persistence contract (§10.8).
type Repository interface {
	Create(op *models.Operation) (*models.Operation, error)
	Update(op *models.Operation) (*models.Operation, error)
	GetByID(ownerUserID, workspaceID string, id uuid.UUID) (*models.Operation, error)
	FindByDedupeKey(ownerUserID, workspaceID, dedupeKey string) (*models.Operation, bool, error)
	List(f Filter) ([]models.Operation, error)
	ListDue(ownerUserID, workspaceID string, limit int) ([]models.Operation, error)
	Dashboard(ownerUserID, workspaceID string) (Dashboard, error)
	AppendEvent(evt *models.OperationEvent) error
	ListEvents(operationID uuid.UUID, limit int) ([]models.OperationEvent, error)
}

// GormRepository is the Postgres-backed Repository.
type GormRepository struct{ DB *gorm.DB }

// NewGormRepository builds a repository over db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{DB: db} }

// DefaultRepository builds a repository over the default DB.
func DefaultRepository() Repository {
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewGormRepository(db)
}

func (r *GormRepository) Create(op *models.Operation) (*models.Operation, error) {
	if err := r.DB.Create(op).Error; err != nil {
		if isActiveDedupeConflict(err) {
			return nil, ErrDuplicateDedupeKey
		}
		return nil, err
	}
	return op, nil
}

func (r *GormRepository) Update(op *models.Operation) (*models.Operation, error) {
	// A background lease serializes writes for Phase 2A; Version is carried for
	// future optimistic concurrency.
	if err := r.DB.Save(op).Error; err != nil {
		return nil, err
	}
	return op, nil
}

func (r *GormRepository) GetByID(ownerUserID, workspaceID string, id uuid.UUID) (*models.Operation, error) {
	var op models.Operation
	err := r.DB.Where("id = ? AND owner_user_id = ? AND workspace_id = ?", id, ownerUserID, workspaceID).First(&op).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (r *GormRepository) FindByDedupeKey(ownerUserID, workspaceID, dedupeKey string) (*models.Operation, bool, error) {
	var op models.Operation
	err := r.DB.
		Where("owner_user_id = ? AND workspace_id = ? AND dedupe_key = ? AND status NOT IN ?", ownerUserID, workspaceID, dedupeKey, []string{string(StatusArchived), string(StatusDismissed)}).
		Order("created_at DESC").First(&op).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &op, true, nil
}

func isActiveDedupeConflict(err error) bool {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" &&
		strings.Contains(postgresError.ConstraintName, "uq_operations_owner_workspace_dedupe_active") {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_operations_owner_workspace_dedupe_active")
}

func (r *GormRepository) List(f Filter) ([]models.Operation, error) {
	q := r.DB.Where("owner_user_id = ? AND workspace_id = ?", f.OwnerUserID, f.WorkspaceID)
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.RiskLevel != "" {
		q = q.Where("risk_level = ?", string(f.RiskLevel))
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var ops []models.Operation
	if err := q.Order("updated_at DESC").Limit(limit).Offset(f.Offset).Find(&ops).Error; err != nil {
		return nil, err
	}
	return ops, nil
}

// actionableStatuses are the statuses the background loop may still progress.
var actionableStatuses = []string{
	string(StatusNew), string(StatusClassified), string(StatusReady),
	string(StatusApproved), string(StatusVerifying), string(StatusWaitingExternal),
	string(StatusInterrupted), string(StatusFailed),
}

func (r *GormRepository) ListDue(ownerUserID, workspaceID string, limit int) ([]models.Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var ops []models.Operation
	err := r.DB.
		Where("owner_user_id = ? AND workspace_id = ? AND status IN ?", ownerUserID, workspaceID, actionableStatuses).
		Where("next_review_at IS NULL OR next_review_at <= now()").
		Order("created_at ASC").Limit(limit).Find(&ops).Error
	if err != nil {
		return nil, err
	}
	return ops, nil
}

func (r *GormRepository) Dashboard(ownerUserID, workspaceID string) (Dashboard, error) {
	d := Dashboard{CountsByStatus: map[string]int{}, CountsByRisk: map[string]int{}}
	type row struct {
		Key string
		N   int
	}
	var byStatus []row
	if err := r.DB.Model(&models.Operation{}).
		Select("status as key, count(*) as n").
		Where("owner_user_id = ? AND workspace_id = ?", ownerUserID, workspaceID).
		Group("status").Scan(&byStatus).Error; err != nil {
		return d, err
	}
	for _, rrow := range byStatus {
		d.CountsByStatus[rrow.Key] = rrow.N
	}
	var byRisk []row
	if err := r.DB.Model(&models.Operation{}).
		Select("risk_level as key, count(*) as n").
		Where("owner_user_id = ? AND workspace_id = ?", ownerUserID, workspaceID).
		Group("risk_level").Scan(&byRisk).Error; err != nil {
		return d, err
	}
	for _, rrow := range byRisk {
		d.CountsByRisk[rrow.Key] = rrow.N
	}
	d.NeedsRobert = d.CountsByStatus[string(StatusAwaitingApproval)]
	d.DoneWhileAway = d.CountsByStatus[string(StatusCompleted)]
	d.Blocked = d.CountsByStatus[string(StatusBlocked)]
	d.Running = d.CountsByStatus[string(StatusRunning)]
	d.Failed = d.CountsByStatus[string(StatusFailed)]

	if err := r.DB.
		Where("owner_user_id = ? AND workspace_id = ?", ownerUserID, workspaceID).
		Order("updated_at DESC").Limit(20).Find(&d.Recent).Error; err != nil {
		return d, err
	}
	return d, nil
}

func (r *GormRepository) AppendEvent(evt *models.OperationEvent) error {
	return r.DB.Create(evt).Error
}

func (r *GormRepository) ListEvents(operationID uuid.UUID, limit int) ([]models.OperationEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var events []models.OperationEvent
	err := r.DB.Where("operation_id = ?", operationID).
		Order("created_at ASC").Limit(limit).Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}
