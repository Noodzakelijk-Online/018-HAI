package resilience

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

const maxResilienceRequestBytes = 64 << 10

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// NewAdvisoryAPI is the self-contained reference assembly for callers that do
// not need to inject a clock. Route registration still requires explicit auth
// and permission guards from the owning router.
func NewAdvisoryAPI(repository Repository) (*Handler, error) {
	if repository == nil {
		return nil, ErrRepositoryUnavailable
	}
	return NewHandler(NewService(repository)), nil
}

// RouteGuards keeps authentication and authorization policy in the owning
// router. RecognizedRole must reject unknown verified role claims.
type RouteGuards struct {
	AuthenticatedOwner gin.HandlerFunc
	RecognizedRole     gin.HandlerFunc
	Read               gin.HandlerFunc
	Write              gin.HandlerFunc
	Govern             gin.HandlerFunc
}

// RegisterRoutes mounts the advisory API below parent. It refuses incomplete
// security wiring and never creates a route with a missing guard.
func RegisterRoutes(parent *gin.RouterGroup, handler *Handler, guards RouteGuards) error {
	if parent == nil || handler == nil || handler.service == nil || handler.service.repository == nil {
		return errors.New("resilience route group and service are required")
	}
	if guards.AuthenticatedOwner == nil || guards.RecognizedRole == nil || guards.Read == nil || guards.Write == nil || guards.Govern == nil {
		return errors.New("resilience routes require authentication, recognized-role, and read, write, and govern permission guards")
	}
	routes := parent.Group("/resilience/workspaces/:workspaceId")
	routes.Use(guards.AuthenticatedOwner, guards.RecognizedRole)
	{
		routes.GET("/status", guards.Read, handler.Status)
		routes.POST("/work-registrations", guards.Write, handler.RegisterWork)
		routes.GET("/leases", guards.Read, handler.ListLeases)
		routes.GET("/leases/:workId", guards.Read, handler.GetLease)
		routes.POST("/leases/:workId/acquire", guards.Write, handler.AcquireLease)
		routes.POST("/leases/:workId/heartbeat", guards.Write, handler.HeartbeatLease)
		routes.POST("/leases/:workId/release", guards.Write, handler.ReleaseLease)
		routes.GET("/workers", guards.Read, handler.ListWorkers)
		routes.GET("/workers/:workerId", guards.Read, handler.GetWorker)
		routes.PUT("/workers/:workerId/heartbeat", guards.Write, handler.RecordWorkerHeartbeat)
		routes.GET("/retries", guards.Read, handler.ListRetries)
		routes.GET("/retries/:workId", guards.Read, handler.GetRetry)
		routes.POST("/retries/:workId/advise", guards.Govern, handler.AdviseRetry)
		routes.POST("/retries/:workId/decide", guards.Govern, handler.AdviseRetry)
		routes.GET("/circuits", guards.Read, handler.ListCircuits)
		routes.GET("/circuits/:circuitId", guards.Read, handler.GetCircuit)
		routes.POST("/circuits/:circuitId/before-attempt", guards.Govern, handler.BeforeCircuit)
		routes.POST("/circuits/:circuitId/observations", guards.Govern, handler.ObserveCircuit)
		routes.GET("/recoveries", guards.Read, handler.ListRecoveries)
		routes.GET("/recoveries/:workId", guards.Read, handler.GetRecovery)
		routes.POST("/recoveries/:workId/advise", guards.Govern, handler.AdviseRecovery)
		routes.GET("/events", guards.Read, handler.ListEvents)
	}
	return nil
}

type workRegistrationBody struct {
	WorkID      string `json:"workId"`
	Operation   string `json:"operation"`
	SourceRef   string `json:"sourceRef"`
	PayloadHash string `json:"payloadHash"`
}

type acquireLeaseBody struct {
	IdempotencyKey string `json:"idempotencyKey"`
	PayloadHash    string `json:"payloadHash"`
	WorkerID       string `json:"workerId"`
	TTL            string `json:"ttl,omitempty"`
	TTLSeconds     int64  `json:"ttlSeconds"`
}

type heartbeatLeaseBody struct {
	WorkerID   string `json:"workerId"`
	Generation uint64 `json:"generation"`
	TTL        string `json:"ttl,omitempty"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

type releaseLeaseBody struct {
	WorkerID   string `json:"workerId"`
	Generation uint64 `json:"generation"`
}

type workerHeartbeatBody struct {
	Sequence uint64 `json:"sequence"`
}

type circuitPolicyBody struct {
	FailureThreshold  uint32 `json:"failureThreshold"`
	OpenDuration      string `json:"openDuration,omitempty"`
	OpenDurationSecs  int64  `json:"openDurationSeconds"`
	MaxHalfOpenProbes uint32 `json:"maxHalfOpenProbes"`
}

type circuitBeforeBody struct {
	Policy circuitPolicyBody `json:"policy"`
}

type circuitObservationBody struct {
	Outcome AttemptOutcome    `json:"outcome"`
	Policy  circuitPolicyBody `json:"policy"`
}

type retryPolicyBody struct {
	MaxAttempts      uint32 `json:"maxAttempts"`
	BaseDelay        string `json:"baseDelay,omitempty"`
	BaseDelaySeconds int64  `json:"baseDelaySeconds"`
	Multiplier       uint32 `json:"multiplier"`
	MaxDelay         string `json:"maxDelay,omitempty"`
	MaxDelaySeconds  int64  `json:"maxDelaySeconds"`
}

type failureBody struct {
	Code    string       `json:"code"`
	Class   FailureClass `json:"class"`
	Message string       `json:"message"`
}

type retryBody struct {
	AttemptsCompleted uint32          `json:"attemptsCompleted"`
	Failure           failureBody     `json:"failure"`
	Policy            retryPolicyBody `json:"policy"`
}

type recoveryBody struct {
	WorkerID            string          `json:"workerId,omitempty"`
	CircuitID           string          `json:"circuitId,omitempty"`
	HeartbeatMaxAge     string          `json:"heartbeatMaxAge,omitempty"`
	HeartbeatMaxAgeSecs int64           `json:"heartbeatMaxAgeSeconds"`
	AttemptsCompleted   uint32          `json:"attemptsCompleted"`
	Failure             *failureBody    `json:"failure,omitempty"`
	RetryPolicy         retryPolicyBody `json:"retryPolicy"`
}

type leaseListResponse struct {
	Leases    []WorkLease       `json:"leases"`
	Authority AuthorityBoundary `json:"authority"`
}

type leaseResponse struct {
	Lease     *WorkLease        `json:"lease"`
	Authority AuthorityBoundary `json:"authority"`
}

type workerListResponse struct {
	Workers   []WorkerHeartbeat `json:"workers"`
	Authority AuthorityBoundary `json:"authority"`
}

type workerResponse struct {
	Worker    *WorkerHeartbeat  `json:"worker"`
	Authority AuthorityBoundary `json:"authority"`
}

type retryListResponse struct {
	Retries   []RetryRecord     `json:"retries"`
	Authority AuthorityBoundary `json:"authority"`
}

type retryResponse struct {
	Retry     *RetryRecord      `json:"retry"`
	Authority AuthorityBoundary `json:"authority"`
}

type circuitListResponse struct {
	Circuits  []CircuitState    `json:"circuits"`
	Authority AuthorityBoundary `json:"authority"`
}

type circuitResponse struct {
	Circuit   *CircuitState     `json:"circuit"`
	Authority AuthorityBoundary `json:"authority"`
}

type recoveryListResponse struct {
	Recoveries []RecoveryRecord  `json:"recoveries"`
	Authority  AuthorityBoundary `json:"authority"`
}

type recoveryResponse struct {
	Recovery  *RecoveryRecord   `json:"recovery"`
	Authority AuthorityBoundary `json:"authority"`
}

type eventListResponse struct {
	Events    []EventRecord     `json:"events"`
	Authority AuthorityBoundary `json:"authority"`
}

type errorResponse struct {
	Error     string            `json:"error"`
	Authority AuthorityBoundary `json:"authority"`
}

func (h *Handler) RegisterWork(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body workRegistrationBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	result, err := h.service.RegisterWork(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, WorkRegistrationInput{
		WorkID: body.WorkID, Operation: body.Operation, SourceRef: body.SourceRef, PayloadHash: body.PayloadHash,
	})
	status := http.StatusOK
	if err == nil && result.Disposition == IdempotencyAccept {
		status = http.StatusCreated
	}
	respondResilience(c, result, err, status)
}

func (h *Handler) Status(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.Status(c.Request.Context(), scope.OwnerID, scope.WorkspaceID)
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ListLeases(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "")
	if !ok {
		return
	}
	result, err := h.service.ListLeases(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, limit)
	respondResilience(c, leaseListResponse{Leases: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) GetLease(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetLease(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, c.Param("workId"))
	respondResilience(c, leaseResponse{Lease: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) AcquireLease(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body acquireLeaseBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	ttl, ok := requestDurationValue(c, body.TTL, body.TTLSeconds, maxLeaseTTL, "invalid resilience lease request")
	if !ok {
		return
	}
	result, err := h.service.AcquireLease(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, LeaseAcquireInput{
		WorkID: c.Param("workId"), WorkerID: body.WorkerID, IdempotencyKey: body.IdempotencyKey,
		PayloadHash: body.PayloadHash, TTL: ttl,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) HeartbeatLease(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body heartbeatLeaseBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	ttl, ok := requestDurationValue(c, body.TTL, body.TTLSeconds, maxLeaseTTL, "invalid resilience lease heartbeat")
	if !ok {
		return
	}
	result, err := h.service.HeartbeatLease(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, LeaseHeartbeatInput{
		WorkID: c.Param("workId"), WorkerID: body.WorkerID, Generation: body.Generation, TTL: ttl,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ReleaseLease(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body releaseLeaseBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	result, err := h.service.ReleaseLease(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, LeaseReleaseInput{
		WorkID: c.Param("workId"), WorkerID: body.WorkerID, Generation: body.Generation,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ListWorkers(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "")
	if !ok {
		return
	}
	result, err := h.service.ListWorkers(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, limit)
	respondResilience(c, workerListResponse{Workers: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) GetWorker(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetWorker(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, c.Param("workerId"))
	respondResilience(c, workerResponse{Worker: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) RecordWorkerHeartbeat(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body workerHeartbeatBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	result, err := h.service.RecordWorkerHeartbeat(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, WorkerHeartbeatInput{
		WorkerID: c.Param("workerId"), Sequence: body.Sequence,
	})
	respondResilience(c, workerResponse{Worker: &result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) ListRetries(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "workId")
	if !ok {
		return
	}
	result, err := h.service.ListRetries(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, strings.TrimSpace(c.Query("workId")), limit)
	respondResilience(c, retryListResponse{Retries: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) GetRetry(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetRetry(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, c.Param("workId"))
	respondResilience(c, retryResponse{Retry: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) AdviseRetry(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body retryBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	policy, ok := retryPolicyFromBody(c, body.Policy)
	if !ok {
		return
	}
	result, err := h.service.AdviseRetry(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, RetryAdvisoryInput{
		WorkID: c.Param("workId"), AttemptsCompleted: body.AttemptsCompleted,
		FailureCode: body.Failure.Code, FailureClass: body.Failure.Class, FailureMessage: body.Failure.Message,
		Policy: policy,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ListCircuits(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "")
	if !ok {
		return
	}
	result, err := h.service.ListCircuits(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, limit)
	respondResilience(c, circuitListResponse{Circuits: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) GetCircuit(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetCircuit(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, c.Param("circuitId"))
	respondResilience(c, circuitResponse{Circuit: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) BeforeCircuit(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body circuitBeforeBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	policy, ok := circuitPolicyFromBody(c, body.Policy)
	if !ok {
		return
	}
	result, err := h.service.BeforeCircuit(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, CircuitBeforeInput{
		CircuitID: c.Param("circuitId"), Policy: policy,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ObserveCircuit(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body circuitObservationBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	policy, ok := circuitPolicyFromBody(c, body.Policy)
	if !ok {
		return
	}
	result, err := h.service.ObserveCircuit(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, CircuitObservationInput{
		CircuitID: c.Param("circuitId"), Outcome: body.Outcome, Policy: policy,
	})
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ListRecoveries(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "workId")
	if !ok {
		return
	}
	result, err := h.service.ListRecoveries(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, strings.TrimSpace(c.Query("workId")), limit)
	respondResilience(c, recoveryListResponse{Recoveries: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) GetRecovery(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	result, err := h.service.GetRecovery(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, c.Param("workId"))
	respondResilience(c, recoveryResponse{Recovery: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func (h *Handler) AdviseRecovery(c *gin.Context) {
	scope, ok := resilienceScope(c)
	if !ok {
		return
	}
	var body recoveryBody
	if !decodeResilienceJSON(c, &body) {
		return
	}
	heartbeatMaxAge, ok := requestDurationValue(c, body.HeartbeatMaxAge, body.HeartbeatMaxAgeSecs, maxHeartbeatAge, "invalid resilience recovery request")
	if !ok {
		return
	}
	retryPolicy, ok := retryPolicyFromBody(c, body.RetryPolicy)
	if !ok {
		return
	}
	input := RecoveryAdvisoryInput{
		WorkID: c.Param("workId"), WorkerID: body.WorkerID, CircuitID: body.CircuitID,
		HeartbeatMaxAge: heartbeatMaxAge, AttemptsCompleted: body.AttemptsCompleted, RetryPolicy: retryPolicy,
	}
	if body.Failure != nil {
		input.FailureCode = body.Failure.Code
		input.FailureClass = body.Failure.Class
		input.FailureMessage = body.Failure.Message
	}
	result, err := h.service.AdviseRecovery(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, input)
	respondResilience(c, result, err, http.StatusOK)
}

func (h *Handler) ListEvents(c *gin.Context) {
	scope, limit, ok := resilienceListScope(c, "")
	if !ok {
		return
	}
	result, err := h.service.ListEvents(c.Request.Context(), scope.OwnerID, scope.WorkspaceID, limit)
	respondResilience(c, eventListResponse{Events: result, Authority: advisoryBoundary()}, err, http.StatusOK)
}

func resilienceListScope(c *gin.Context, optionalFilter string) (Scope, int, bool) {
	allowed := []string{"limit"}
	if optionalFilter != "" {
		allowed = append(allowed, optionalFilter)
	}
	scope, ok := resilienceScope(c, allowed...)
	if !ok {
		return Scope{}, 0, false
	}
	limit, ok := resilienceLimit(c)
	return scope, limit, ok
}

func resilienceScope(c *gin.Context, allowedQuery ...string) (Scope, bool) {
	allowed := make(map[string]bool, len(allowedQuery))
	for _, key := range allowedQuery {
		allowed[key] = true
	}
	for key := range c.Request.URL.Query() {
		if !allowed[key] {
			writeResilienceError(c, http.StatusBadRequest, "invalid resilience query")
			return Scope{}, false
		}
	}
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		writeResilienceError(c, http.StatusUnauthorized, "an authenticated owner session is required for resilience access")
		return Scope{}, false
	}
	scope := Scope{OwnerID: owner, WorkspaceID: strings.TrimSpace(c.Param("workspaceId"))}
	if err := validateScope(scope); err != nil {
		writeResilienceError(c, http.StatusBadRequest, "invalid resilience workspace")
		return Scope{}, false
	}
	return scope, true
}

func decodeResilienceJSON(c *gin.Context, target any) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeResilienceError(c, http.StatusUnsupportedMediaType, "resilience requests require application/json")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxResilienceRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeResilienceError(c, http.StatusRequestEntityTooLarge, "resilience request is too large")
		} else {
			writeResilienceError(c, http.StatusBadRequest, "invalid resilience request")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeResilienceError(c, http.StatusBadRequest, "resilience request must contain one JSON object")
		return false
	}
	return true
}

func requestDuration(c *gin.Context, seconds int64, maximum time.Duration, message string) (time.Duration, bool) {
	if seconds <= 0 || seconds > int64(maximum/time.Second) {
		writeResilienceError(c, http.StatusBadRequest, message)
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func requestDurationValue(c *gin.Context, encoded string, seconds int64, maximum time.Duration, message string) (time.Duration, bool) {
	encoded = strings.TrimSpace(encoded)
	if encoded != "" && seconds != 0 {
		writeResilienceError(c, http.StatusBadRequest, message)
		return 0, false
	}
	if encoded == "" {
		return requestDuration(c, seconds, maximum, message)
	}
	duration, err := time.ParseDuration(encoded)
	if err != nil || duration <= 0 || duration > maximum {
		writeResilienceError(c, http.StatusBadRequest, message)
		return 0, false
	}
	return duration, true
}

func circuitPolicyFromBody(c *gin.Context, body circuitPolicyBody) (CircuitPolicy, bool) {
	duration, ok := requestDurationValue(c, body.OpenDuration, body.OpenDurationSecs, maxBackoff, "invalid resilience circuit policy")
	if !ok {
		return CircuitPolicy{}, false
	}
	policy := CircuitPolicy{FailureThreshold: body.FailureThreshold, OpenDuration: duration, MaxHalfOpenProbes: body.MaxHalfOpenProbes}
	if err := validateCircuitPolicy(policy); err != nil {
		writeResilienceError(c, http.StatusBadRequest, "invalid resilience circuit policy")
		return CircuitPolicy{}, false
	}
	return policy, true
}

func retryPolicyFromBody(c *gin.Context, body retryPolicyBody) (RetryPolicy, bool) {
	baseDelay, ok := requestDurationValue(c, body.BaseDelay, body.BaseDelaySeconds, maxBackoff, "invalid resilience retry policy")
	if !ok {
		return RetryPolicy{}, false
	}
	maxDelay, ok := requestDurationValue(c, body.MaxDelay, body.MaxDelaySeconds, maxBackoff, "invalid resilience retry policy")
	if !ok {
		return RetryPolicy{}, false
	}
	policy := RetryPolicy{MaxAttempts: body.MaxAttempts, BaseDelay: baseDelay, Multiplier: body.Multiplier, MaxDelay: maxDelay}
	if err := validateRetryPolicy(policy); err != nil {
		writeResilienceError(c, http.StatusBadRequest, "invalid resilience retry policy")
		return RetryPolicy{}, false
	}
	return policy, true
}

func resilienceLimit(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return DefaultHistoryLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > MaxHistoryLimit {
		writeResilienceError(c, http.StatusBadRequest, "invalid resilience history limit")
		return 0, false
	}
	return limit, true
}

func respondResilience(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}
	switch {
	case errors.Is(err, ErrStateNotFound):
		writeResilienceError(c, http.StatusNotFound, "resilience state not found")
	case errors.Is(err, ErrStateConflict), errors.Is(err, ErrStaleFence):
		writeResilienceError(c, http.StatusConflict, "resilience state conflict")
	case errors.Is(err, ErrRepositoryUnavailable):
		writeResilienceError(c, http.StatusServiceUnavailable, "resilience service is unavailable")
	default:
		writeResilienceError(c, http.StatusBadRequest, "resilience request was rejected")
	}
}

func writeResilienceError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: message, Authority: advisoryBoundary()})
}
