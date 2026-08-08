package infra

import (
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/migrations"
	"fmt"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewPostgresDatabase(user, password, dbName, dbHost string, dbPort int) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=UTC",
		dbHost, user, password, dbName, dbPort)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// OpenDefaultDB opens the configured database without running migrations. Use it
// for read-only operations (e.g. `migrate status`) or explicit rollbacks.
func OpenDefaultDB() (*gorm.DB, error) {
	return NewPostgresDatabase(config.AppConfig.DbUser, config.AppConfig.DbPassword,
		config.AppConfig.DbName, config.AppConfig.DbHost, config.AppConfig.DbPort)
}

func GetDefaultDB() (*gorm.DB, error) {
	db, err := OpenDefaultDB()
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db); err != nil {
		return nil, err
	}

	return db, nil
}

// autoMigrateEnabled reports whether Gorm AutoMigrate should run for table
// creation. It defaults to FALSE: the versioned migrations — including the
// generated baseline in migrations/pre/0002_baseline — are the source of truth
// for the schema, so production never mutates its own schema implicitly.
//
// Set DB_AUTOMIGRATE=true only in development, to let Gorm materialise a new
// model before you regenerate the baseline migration for it.
func autoMigrateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DB_AUTOMIGRATE"))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func RunMigrations(db *gorm.DB) error {
	// Phase 1: versioned migrations that must precede table creation (extensions
	// the models' UUID defaults depend on).
	if _, err := ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		return fmt.Errorf("apply pre migrations: %w", err)
	}
	// pgvector is opt-in. Normal Postgres deployments remain usable when no
	// local embedding endpoint has been reviewed; an enabled deployment fails
	// early if its database image does not contain the extension.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_SEMANTIC_RETRIEVAL_ENABLED")), "true") {
		if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
			return fmt.Errorf("enable pgvector extension: %w", err)
		}
	}

	// Phase 2: optional dev-only table sync. The baseline migration already
	// created every table, so this is opt-in (DB_AUTOMIGRATE=true) and exists
	// only to materialise a newly-added model before its migration is generated.
	if autoMigrateEnabled() {
		if err := db.AutoMigrate(
			&models.Automation{},
			&models.AutomationHealthEvent{},
			&models.AutomationLaunchEvent{},
			&models.AutomationDependency{},
			&models.AutomationRouteCheck{},
			&models.AutomationAlert{},
			&models.AutomationIncident{},
			&models.AutomationSLO{},
			&models.LLMProviderProbe{},
			&models.ContextMemory{},
			&models.AIConversationArchive{},
			&models.AIMemoryInsight{},
			&models.SourceConnector{},
			&models.ConnectedSource{},
			&models.SourceSyncJob{},
			&models.SourceRawItem{},
			&models.SourceExtraction{},
			&models.SourceIndexEntry{},
			&models.SourceAuditLog{},
			&models.SourceOAuthToken{},
			&models.VerificationRun{},
			&models.VerificationEvidence{},
			&models.VerificationClaim{},
			&models.VerificationAuditLog{},
			&models.WorkflowItem{},
			&models.WorkflowChecklistItem{},
			&models.WorkflowIntakeRecord{},
			&models.WorkflowProjectMatch{},
			&models.WorkflowEvidenceClaim{},
			&models.WorkflowOpenLoop{},
			&models.WorkflowProposal{},
			&models.WorkflowQualityGate{},
			&models.WorkflowRule{},
			&models.WorkflowTransition{},
			&models.WorkflowSourceLink{},
			&models.WorkflowDecision{},
			&models.WorkflowEvent{},
			&models.WorkflowCompletionAttestation{},
			&models.WorkflowReminderActivationRequest{},
			&models.WorkflowReminderActivationDecision{},
			&models.Pursuit{},
			&models.PursuitLink{},
			&models.PursuitActivity{},
			&models.PursuitTaskAttempt{},
			&models.PursuitPortfolioWorkflowSettlementProof{},
			&models.AmbientNeed{},
			&models.AmbientNeedOverride{},
			&models.AmbientOpportunity{},
			&models.AmbientScan{},
			&models.AutonomyWorldState{},
			&models.AutonomyActionTrace{},
			&models.AutonomyEvaluation{},
			&models.AutonomyStressRun{},
			// Phase 2 — Operation Ledger (§7/§10.5).
			&models.Operation{},
			&models.OperationEvent{},
			// Phase 2 — durable model telemetry (§18/§10.9).
			&models.ModelRunTelemetry{},
			&models.OptimizationProposalRun{},
			&models.TemporalWorkflowRun{},
			&models.BrowserVerificationRun{},
			&models.WASIRun{},
			// Durable worker: background jobs that survive a restart.
			&models.DurableJob{},
			// Owner-scoped Framework Registry preferences, immutable selection
			// audits, and versioned Robert Constitution records.
			&models.FrameworkPreference{},
			&models.FrameworkSelectionRecord{},
			&models.RobertConstitutionVersion{},
			// Owner-scoped, append-only task completion and approval state.
			&models.TaskOperationRecord{},
			&models.TaskCompletionPlanLog{},
			&models.TaskReviewItemRecord{},
			&models.TaskReviewDecisionRecord{},
			// Owner-scoped whole-life ontology, append-only need/capacity
			// observations, and durable goal hierarchy.
			&models.LifeEntityDomainLink{},
			&models.LifeNeedObservation{},
			&models.LifeCapacitySnapshot{},
			&models.LifeGoalNode{},
			&models.LifePriorityAssessment{},
			&models.StandingMandate{},
			&models.StandingMandateDecision{},
			&models.DomainPackPreference{},
		); err != nil {
			return err
		}
	}
	// Phase 3: versioned migrations that depend on the tables existing (indexes,
	// constraints, backfills). These replace the ad-hoc db.Exec DDL that used to
	// live here, so every schema change is now a reviewable, recorded migration.
	if _, err := ApplyMigrations(db, migrations.Files, "post"); err != nil {
		return fmt.Errorf("apply post migrations: %w", err)
	}
	return nil
}
