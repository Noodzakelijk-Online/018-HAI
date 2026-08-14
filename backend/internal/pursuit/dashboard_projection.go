package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// dashboardDetailsForOwner expands every active pursuit from one bounded set
// of repository reads. It returns false when the repository does not implement
// the optimized contract so callers can retain the proven detail path.
func (s *service) dashboardDetailsForOwner(ownerIdentity string, pursuits []models.Pursuit) (map[uuid.UUID]*PursuitDetail, bool, error) {
	bulk, ok := s.repo.(pursuitDashboardBulkRepository)
	if !ok {
		return nil, false, nil
	}
	active := make([]models.Pursuit, 0, len(pursuits))
	ids := make([]uuid.UUID, 0, len(pursuits))
	for _, pursuit := range pursuits {
		if pursuitClosed(pursuit) {
			continue
		}
		active = append(active, pursuit)
		ids = append(ids, pursuit.ID)
	}
	result := make(map[uuid.UUID]*PursuitDetail, len(active))
	if len(active) == 0 {
		return result, true, nil
	}

	links, err := bulk.FindVisibleLinksForPursuits(ownerIdentity, ids)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard links", err)
	}
	activity, err := bulk.FindActivitiesForPursuits(ids, 50)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard activity", err)
	}
	taskAttempts, err := bulk.FindTaskAttemptsForPursuits(ownerIdentity, ids, 20)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard task attempts", err)
	}

	workflowIDs := linkUUIDs(links, LinkWorkflow)
	memoryIDs := linkUUIDs(links, LinkMemory)
	conversationIDs := linkUUIDs(links, LinkAIConversation)
	ambientIDs := linkUUIDs(links, LinkAmbientOpportunity)
	sourceItemIDs := linkUUIDs(links, LinkSourceItem)
	extractionIDs := linkUUIDs(links, LinkSourceExtraction)
	verificationIDs := linkUUIDs(links, LinkVerification)
	linkedAutomationIDs := linkUUIDs(links, LinkAutomation)
	linkedRuntimeIDs := linkUUIDs(links, LinkAgentRuntime)

	workflows, err := s.repo.FindLinkedWorkflows(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard workflows", err)
	}
	checklist, err := s.repo.FindLinkedChecklistItems(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard checklist", err)
	}
	openLoops, err := s.repo.FindLinkedOpenLoops(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard open loops", err)
	}
	proposals, err := s.repo.FindLinkedProposals(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard proposals", err)
	}
	qualityGates, err := s.repo.FindLinkedQualityGates(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard quality gates", err)
	}
	decisions, err := s.repo.FindLinkedDecisions(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard decisions", err)
	}
	transitions, err := s.repo.FindLinkedTransitions(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard transitions", err)
	}
	sourceLinks, err := s.repo.FindLinkedSourceLinks(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard source links", err)
	}
	events, err := s.repo.FindLinkedEvents(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard events", err)
	}
	evidence, err := s.repo.FindLinkedEvidence(workflowIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard evidence", err)
	}
	memories, err := s.repo.FindLinkedMemories(memoryIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard memories", err)
	}
	conversations, err := s.repo.FindLinkedConversations(conversationIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard conversations", err)
	}
	ambient, err := s.repo.FindLinkedAmbientOpportunities(ambientIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard ambient opportunities", err)
	}
	sourceItems, err := s.repo.FindLinkedSourceItems(sourceItemIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard source items", err)
	}
	extractions, err := s.repo.FindLinkedExtractions(extractionIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard source extractions", err)
	}
	verificationRuns, err := s.repo.FindLinkedVerificationRuns(verificationIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard verification runs", err)
	}
	allVerificationRunIDs := verificationRunIDs(verificationRuns)
	verificationClaims, err := s.repo.FindLinkedVerificationClaims(allVerificationRunIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard verification claims", err)
	}
	verificationEvidence, err := s.repo.FindLinkedVerificationEvidence(allVerificationRunIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard verification evidence", err)
	}
	automationIDs := uniqueUUIDs(append(linkedAutomationIDs, workflowAutomationIDs(workflows)...))
	automations, err := s.repo.FindLinkedAutomations(automationIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard automations", err)
	}
	runtimeAttempts, err := bulk.FindRuntimeAttemptsForOwner(ownerIdentity, automationIDs, linkedRuntimeIDs)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard runtime attempts", err)
	}
	resources, err := bulk.FindResourceProjectionForPursuits(ownerIdentity, active)
	if err != nil {
		return nil, true, pursuitDetailLoadError("dashboard resource ledger", err)
	}

	for _, pursuit := range active {
		pursuitLinks := filterRecords(links, []uuid.UUID{pursuit.ID}, func(item models.PursuitLink) uuid.UUID { return item.PursuitID })
		pursuitWorkflowIDs := linkUUIDs(pursuitLinks, LinkWorkflow)
		pursuitWorkflows := filterRecords(workflows, pursuitWorkflowIDs, func(item models.WorkflowItem) uuid.UUID { return item.ID })
		pursuitAutomationIDs := uniqueUUIDs(append(linkUUIDs(pursuitLinks, LinkAutomation), workflowAutomationIDs(pursuitWorkflows)...))
		pursuitRuntimeIDs := linkUUIDs(pursuitLinks, LinkAgentRuntime)
		pursuitVerificationRuns := filterRecords(verificationRuns, linkUUIDs(pursuitLinks, LinkVerification), func(item models.VerificationRun) uuid.UUID { return item.ID })
		pursuitRunIDs := verificationRunIDs(pursuitVerificationRuns)
		pursuitRuntimeAttempts := filterRuntimeAttempts(runtimeAttempts, pursuitAutomationIDs, pursuitRuntimeIDs, 20)

		usage := dashboardResourceUsage(ownerIdentity, pursuit, resources)
		records := pursuitDetailRecords{
			Links:                pursuitLinks,
			Activity:             filterRecords(activity, []uuid.UUID{pursuit.ID}, func(item models.PursuitActivity) uuid.UUID { return item.PursuitID }),
			TaskAttempts:         filterRecords(taskAttempts, []uuid.UUID{pursuit.ID}, func(item models.PursuitTaskAttempt) uuid.UUID { return item.PursuitID }),
			Workflows:            pursuitWorkflows,
			ChecklistItems:       filterRecords(checklist, pursuitWorkflowIDs, func(item models.WorkflowChecklistItem) uuid.UUID { return item.WorkflowID }),
			OpenLoops:            filterRecords(openLoops, pursuitWorkflowIDs, func(item models.WorkflowOpenLoop) uuid.UUID { return item.WorkflowID }),
			Proposals:            filterRecords(proposals, pursuitWorkflowIDs, func(item models.WorkflowProposal) uuid.UUID { return item.WorkflowID }),
			QualityGates:         filterRecords(qualityGates, pursuitWorkflowIDs, func(item models.WorkflowQualityGate) uuid.UUID { return item.WorkflowID }),
			Decisions:            filterRecords(decisions, pursuitWorkflowIDs, func(item models.WorkflowDecision) uuid.UUID { return item.WorkflowID }),
			Transitions:          filterRecords(transitions, pursuitWorkflowIDs, func(item models.WorkflowTransition) uuid.UUID { return item.WorkflowID }),
			SourceLinks:          filterRecords(sourceLinks, pursuitWorkflowIDs, func(item models.WorkflowSourceLink) uuid.UUID { return item.WorkflowID }),
			Events:               filterRecords(events, pursuitWorkflowIDs, func(item models.WorkflowEvent) uuid.UUID { return item.WorkflowID }),
			Evidence:             filterRecords(evidence, pursuitWorkflowIDs, func(item models.WorkflowEvidenceClaim) uuid.UUID { return item.WorkflowID }),
			Memories:             filterRecords(memories, linkUUIDs(pursuitLinks, LinkMemory), func(item models.ContextMemory) uuid.UUID { return item.ID }),
			Conversations:        filterRecords(conversations, linkUUIDs(pursuitLinks, LinkAIConversation), func(item models.AIConversationArchive) uuid.UUID { return item.ID }),
			AmbientOpportunities: filterRecords(ambient, linkUUIDs(pursuitLinks, LinkAmbientOpportunity), func(item models.AmbientOpportunity) uuid.UUID { return item.ID }),
			SourceItems:          filterRecords(sourceItems, linkUUIDs(pursuitLinks, LinkSourceItem), func(item models.SourceRawItem) uuid.UUID { return item.ID }),
			SourceExtractions:    filterRecords(extractions, linkUUIDs(pursuitLinks, LinkSourceExtraction), func(item models.SourceExtraction) uuid.UUID { return item.ID }),
			VerificationRuns:     pursuitVerificationRuns,
			VerificationClaims:   filterRecords(verificationClaims, pursuitRunIDs, func(item models.VerificationClaim) uuid.UUID { return item.RunID }),
			VerificationEvidence: filterRecords(verificationEvidence, pursuitRunIDs, func(item models.VerificationEvidence) uuid.UUID { return item.RunID }),
			Automations:          filterRecords(automations, pursuitAutomationIDs, func(item models.Automation) uuid.UUID { return item.ID }),
			RuntimeAttempts:      pursuitRuntimeAttempts,
			ResourceUsage:        usage,
		}
		result[pursuit.ID] = s.buildPursuitDetail(pursuit, records)
	}
	return result, true, nil
}

func filterRecords[T any](items []T, ids []uuid.UUID, id func(T) uuid.UUID) []T {
	result := []T{}
	if len(items) == 0 || len(ids) == 0 {
		return result
	}
	wanted := make(map[uuid.UUID]bool, len(ids))
	for _, value := range ids {
		wanted[value] = true
	}
	for _, item := range items {
		if wanted[id(item)] {
			result = append(result, item)
		}
	}
	return result
}

func filterRuntimeAttempts(items []models.AutomationLaunchEvent, automationIDs, launchIDs []uuid.UUID, limit int) []models.AutomationLaunchEvent {
	automations := make(map[uuid.UUID]bool, len(automationIDs))
	launches := make(map[uuid.UUID]bool, len(launchIDs))
	for _, id := range automationIDs {
		automations[id] = true
	}
	for _, id := range launchIDs {
		launches[id] = true
	}
	result := []models.AutomationLaunchEvent{}
	for _, item := range items {
		if !automations[item.AutomationID] && !launches[item.ID] {
			continue
		}
		result = append(result, item)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func dashboardResourceUsage(ownerIdentity string, pursuit models.Pursuit, projection pursuitDashboardResourceProjection) PursuitResourceUsage {
	configured := pursuit.ResourceLimits.MaxEffortHours > 0 || pursuit.ResourceLimits.MaxSpendEUR > 0
	if !configured {
		return PursuitResourceUsage{State: "not_configured", Reservations: []PursuitActiveResourceReservation{}, EffortLimitHours: pursuit.ResourceLimits.MaxEffortHours, SpendLimitEUR: pursuit.ResourceLimits.MaxSpendEUR}
	}
	if strings.TrimSpace(firstNonEmpty(ownerIdentity, pursuit.OwnerIdentity)) == "" {
		return PursuitResourceUsage{State: "unavailable", LimitsConfigured: true, Reservations: []PursuitActiveResourceReservation{}, EffortLimitHours: pursuit.ResourceLimits.MaxEffortHours, SpendLimitEUR: pursuit.ResourceLimits.MaxSpendEUR, BlockingReason: "resource usage cannot be verified; new work is paused while a pursuit ceiling is configured"}
	}
	return pursuitResourceUsageFromRecords(pursuit, projection.Totals[pursuit.ID], projection.ReservationTotals[pursuit.ID], projection.ActiveReservations[pursuit.ID], time.Now().UTC())
}

func dashboardProjectionMissing(id uuid.UUID) error {
	return fmt.Errorf("dashboard projection did not contain pursuit %s", id)
}
