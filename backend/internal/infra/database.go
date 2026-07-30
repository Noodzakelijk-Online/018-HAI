package infra

import (
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/models"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"strings"
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

func GetDefaultDB() (*gorm.DB, error) {
	db, err := NewPostgresDatabase(config.AppConfig.DbUser, config.AppConfig.DbPassword,
		config.AppConfig.DbName, config.AppConfig.DbHost, config.AppConfig.DbPort)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db); err != nil {
		return nil, err
	}

	return db, nil
}

func RunMigrations(db *gorm.DB) error {
	// uuid_generate_v4() is used as the default for primary keys.
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
		return err
	}
	// pgvector is opt-in. Normal Postgres deployments remain usable when no
	// local embedding endpoint has been reviewed; an enabled deployment fails
	// early if its database image does not contain the extension.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_SEMANTIC_RETRIEVAL_ENABLED")), "true") {
		if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
			return fmt.Errorf("enable pgvector extension: %w", err)
		}
	}
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
		&models.LLMModelMaintenance{},
		&models.LLMGenerationRecord{},
		&models.BrainCatalogUpstreamReview{},
		&models.BrainCatalogCollectionReview{},
		&models.BrainCatalogRepositoryDiscoveryReview{},
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
		&models.Pursuit{},
		&models.PursuitLink{},
		&models.PursuitActivity{},
		&models.PursuitTaskAttempt{},
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
		&models.MiniSWEPatchProposal{},
	); err != nil {
		return err
	}
	// Conversation identities were originally global. Keep legacy ownerless
	// imports readable, but make new records unique per authenticated owner so
	// two local HAI users cannot overwrite each other's imported history.
	if err := db.Exec(`DROP INDEX IF EXISTS idx_ai_conversation_identity`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_conversation_owner_identity ON ai_conversation_archives (owner_identity, platform, external_id)`).Error; err != nil {
		return err
	}
	return nil
}
