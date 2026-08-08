package controlledlearning

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type rollbackApplicationHTTPReq struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion string `json:"expectedVersion"`
	HumanConfirmed  bool   `json:"humanConfirmed"`
	Rationale       string `json:"rationale"`
}

func (handler *Handler) ListApplications(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	limit, ok := controlledLearningLimit(c)
	if !ok {
		return
	}
	proposalID := strings.TrimSpace(c.Query("proposalId"))
	if proposalID != "" {
		if err := validateRequired("proposal id", proposalID, maxIdentifierLength); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning list filter"})
			return
		}
	}
	status := ApplicationStatus(strings.TrimSpace(c.Query("status")))
	if status != "" && !validApplicationFilterStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning list filter"})
		return
	}

	applications, err := handler.service.ListApplications(c.Request.Context(), ApplicationQuery{
		OwnerIdentity: owner,
		ProposalID:    proposalID,
		Status:        status,
		Limit:         limit,
	})
	if err != nil {
		respondControlledLearning(c, nil, err, http.StatusOK)
		return
	}
	for index := range applications {
		applications[index] = publicApplicationRecord(applications[index])
	}
	c.JSON(http.StatusOK, gin.H{"applications": applications})
}

func (handler *Handler) GetApplication(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	applicationID, ok := controlledLearningPathID(c, "id", "application id")
	if !ok {
		return
	}
	application, err := handler.service.GetApplication(
		c.Request.Context(),
		owner,
		applicationID,
	)
	respondControlledLearning(
		c,
		publicApplicationRecord(application),
		err,
		http.StatusOK,
	)
}

func (handler *Handler) ListApplicationEvents(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	applicationID, ok := controlledLearningPathID(c, "id", "application id")
	if !ok {
		return
	}
	limit, ok := controlledLearningLimit(c)
	if !ok {
		return
	}
	events, err := handler.service.ListApplicationEvents(
		c.Request.Context(),
		owner,
		applicationID,
	)
	if err != nil {
		respondControlledLearning(c, nil, err, http.StatusOK)
		return
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func (handler *Handler) RollbackApplication(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	applicationID, ok := controlledLearningPathID(c, "id", "application id")
	if !ok {
		return
	}
	var input rollbackApplicationHTTPReq
	if !decodeControlledLearningJSON(c, &input) {
		return
	}
	if !input.HumanConfirmed {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "controlled learning rollback requires explicit human confirmation",
		})
		return
	}
	for label, value := range map[string]string{
		"rollback idempotency key": input.IdempotencyKey,
		"expected applied version": input.ExpectedVersion,
		"rollback rationale":       input.Rationale,
	} {
		maximum := maxDetailLength
		if label == "rollback idempotency key" {
			maximum = maxIdentifierLength
		}
		if err := validateRequired(label, value, maximum); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "controlled learning rollback request failed validation",
			})
			return
		}
	}

	application, err := handler.service.Rollback(c.Request.Context(), RollbackRequest{
		OwnerIdentity:   owner,
		ApplicationID:   applicationID,
		IdempotencyKey:  strings.TrimSpace(input.IdempotencyKey),
		ActorIdentity:   owner,
		HumanConfirmed:  true,
		Rationale:       strings.TrimSpace(input.Rationale),
		ExpectedVersion: strings.TrimSpace(input.ExpectedVersion),
	})
	respondControlledLearning(
		c,
		publicApplicationRecord(application),
		err,
		http.StatusOK,
	)
}

func validApplicationFilterStatus(status ApplicationStatus) bool {
	switch status {
	case ApplicationApplying,
		ApplicationApplied,
		ApplicationHandoffPending,
		ApplicationHandoffReady,
		ApplicationFailed,
		ApplicationRollbackApplying,
		ApplicationRolledBack,
		ApplicationRollbackFailed:
		return true
	default:
		return false
	}
}

func publicApplicationRecord(application ApplicationRecord) ApplicationRecord {
	application.RollbackToken = ""
	return application
}

func publicDecisionResult(result DecisionResult) DecisionResult {
	if result.Application == nil {
		return result
	}
	application := publicApplicationRecord(*result.Application)
	result.Application = &application
	return result
}
