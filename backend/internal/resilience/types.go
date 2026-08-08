// Package resilience provides deterministic, advisory-only state transitions
// for HAI's distributed reliability control plane.
//
// The package does not run work, call workers, persist records, consume an
// approval, or grant execution authority. Callers are responsible for
// authenticating the supplied scope and atomically persisting accepted state
// transitions.
package resilience

import "time"

const ContractVersion = 1

const AuthorityAdvisoryOnly = "advisory_only"

// AuthorityBoundary is embedded in every decision returned by this package.
// The false fields are intentional and must not be interpreted as permissions.
type AuthorityBoundary struct {
	Mode             string `json:"mode"`
	CanExecute       bool   `json:"canExecute"`
	GrantsAuthority  bool   `json:"grantsAuthority"`
	ConsumesApproval bool   `json:"consumesApproval"`
	DispatchesWork   bool   `json:"dispatchesWork"`
}

func advisoryBoundary() AuthorityBoundary {
	return AuthorityBoundary{Mode: AuthorityAdvisoryOnly}
}

// Scope is the mandatory isolation boundary for all durable records.
type Scope struct {
	OwnerID     string `json:"ownerId"`
	WorkspaceID string `json:"workspaceId"`
}

type WorkDescriptor struct {
	ContractVersion int       `json:"contractVersion"`
	Scope           Scope     `json:"scope"`
	WorkID          string    `json:"workId"`
	IdempotencyKey  string    `json:"idempotencyKey"`
	PayloadHash     string    `json:"payloadHash"`
	CreatedAt       time.Time `json:"createdAt"`
}

type IdempotencyRecord struct {
	ContractVersion int       `json:"contractVersion"`
	Scope           Scope     `json:"scope"`
	WorkID          string    `json:"workId"`
	IdempotencyKey  string    `json:"idempotencyKey"`
	PayloadHash     string    `json:"payloadHash"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type IdempotencyDisposition string

const (
	IdempotencyAccept    IdempotencyDisposition = "accept"
	IdempotencyDuplicate IdempotencyDisposition = "duplicate"
)

type IdempotencyDecision struct {
	Disposition     IdempotencyDisposition `json:"disposition"`
	CanonicalWorkID string                 `json:"canonicalWorkId"`
	Record          *IdempotencyRecord     `json:"record,omitempty"`
	Reason          string                 `json:"reason"`
	Authority       AuthorityBoundary      `json:"authority"`
}

type LeaseState string

const (
	LeaseActive   LeaseState = "active"
	LeaseReleased LeaseState = "released"
)

// WorkLease is a fencing-token lease intended for durable persistence.
// Generation must be checked by any downstream executor; the lease itself is
// not permission to execute.
type WorkLease struct {
	ContractVersion int        `json:"contractVersion"`
	Scope           Scope      `json:"scope"`
	WorkID          string     `json:"workId"`
	IdempotencyKey  string     `json:"idempotencyKey"`
	PayloadHash     string     `json:"payloadHash"`
	WorkerID        string     `json:"workerId"`
	Generation      uint64     `json:"generation"`
	State           LeaseState `json:"state"`
	AcquiredAt      time.Time  `json:"acquiredAt"`
	LastHeartbeatAt time.Time  `json:"lastHeartbeatAt"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	ReleasedAt      *time.Time `json:"releasedAt,omitempty"`
}

type LeaseRequest struct {
	ContractVersion int           `json:"contractVersion"`
	Scope           Scope         `json:"scope"`
	WorkID          string        `json:"workId"`
	IdempotencyKey  string        `json:"idempotencyKey"`
	PayloadHash     string        `json:"payloadHash"`
	WorkerID        string        `json:"workerId"`
	Now             time.Time     `json:"now"`
	TTL             time.Duration `json:"ttl"`
}

type LeaseDisposition string

const (
	LeaseGrant     LeaseDisposition = "grant"
	LeaseReclaim   LeaseDisposition = "reclaim"
	LeaseBusy      LeaseDisposition = "busy"
	LeaseDuplicate LeaseDisposition = "duplicate"
	LeaseRenew     LeaseDisposition = "renew"
	LeaseRelease   LeaseDisposition = "release"
)

type LeaseDecision struct {
	Disposition LeaseDisposition  `json:"disposition"`
	Lease       *WorkLease        `json:"lease,omitempty"`
	Reason      string            `json:"reason"`
	Authority   AuthorityBoundary `json:"authority"`
}

type LeaseHeartbeat struct {
	ContractVersion int           `json:"contractVersion"`
	Scope           Scope         `json:"scope"`
	WorkID          string        `json:"workId"`
	WorkerID        string        `json:"workerId"`
	Generation      uint64        `json:"generation"`
	ObservedAt      time.Time     `json:"observedAt"`
	TTL             time.Duration `json:"ttl"`
}

type WorkerHeartbeat struct {
	ContractVersion int       `json:"contractVersion"`
	Scope           Scope     `json:"scope"`
	WorkerID        string    `json:"workerId"`
	Sequence        uint64    `json:"sequence"`
	ObservedAt      time.Time `json:"observedAt"`
}

type HeartbeatStatus string

const (
	HeartbeatHealthy HeartbeatStatus = "healthy"
	HeartbeatStale   HeartbeatStatus = "stale"
	HeartbeatMissing HeartbeatStatus = "missing"
)

type HeartbeatDecision struct {
	Status    HeartbeatStatus   `json:"status"`
	Age       time.Duration     `json:"age"`
	Reason    string            `json:"reason"`
	Authority AuthorityBoundary `json:"authority"`
}

type FailureClass string

const (
	FailureTransient    FailureClass = "transient"
	FailureRateLimited  FailureClass = "rate_limited"
	FailurePermanent    FailureClass = "permanent"
	FailureInvalidWork  FailureClass = "invalid_work"
	FailureUnauthorized FailureClass = "unauthorized"
	FailureSecurity     FailureClass = "security"
	FailureUnknown      FailureClass = "unknown"
)

type Failure struct {
	Code    string       `json:"code"`
	Class   FailureClass `json:"class"`
	Message string       `json:"message"`
}

type RetryPolicy struct {
	MaxAttempts uint32        `json:"maxAttempts"`
	BaseDelay   time.Duration `json:"baseDelay"`
	Multiplier  uint32        `json:"multiplier"`
	MaxDelay    time.Duration `json:"maxDelay"`
}

type RetryDisposition string

const (
	RetrySchedule   RetryDisposition = "schedule_retry"
	RetryDeadLetter RetryDisposition = "dead_letter"
)

type DeadLetterClass string

const (
	DeadLetterRetryExhausted DeadLetterClass = "retry_exhausted"
	DeadLetterPermanent      DeadLetterClass = "permanent_failure"
	DeadLetterInvalid        DeadLetterClass = "invalid_work"
	DeadLetterUnauthorized   DeadLetterClass = "unauthorized"
	DeadLetterSecurity       DeadLetterClass = "security_violation"
	DeadLetterUnknown        DeadLetterClass = "unknown_failure"
)

type RetryDecision struct {
	Scope             Scope             `json:"scope"`
	WorkID            string            `json:"workId"`
	Disposition       RetryDisposition  `json:"disposition"`
	AttemptsCompleted uint32            `json:"attemptsCompleted"`
	RetryAt           *time.Time        `json:"retryAt,omitempty"`
	DeadLetterClass   DeadLetterClass   `json:"deadLetterClass,omitempty"`
	Failure           Failure           `json:"failure"`
	Reason            string            `json:"reason"`
	Authority         AuthorityBoundary `json:"authority"`
}

type CircuitPhase string

const (
	CircuitClosed   CircuitPhase = "closed"
	CircuitOpen     CircuitPhase = "open"
	CircuitHalfOpen CircuitPhase = "half_open"
)

type CircuitState struct {
	ContractVersion     int          `json:"contractVersion"`
	Scope               Scope        `json:"scope"`
	CircuitID           string       `json:"circuitId"`
	Phase               CircuitPhase `json:"phase"`
	ConsecutiveFailures uint32       `json:"consecutiveFailures"`
	ProbesInFlight      uint32       `json:"probesInFlight"`
	OpenedAt            *time.Time   `json:"openedAt,omitempty"`
	RetryAfter          *time.Time   `json:"retryAfter,omitempty"`
	Revision            uint64       `json:"revision"`
}

type CircuitPolicy struct {
	FailureThreshold  uint32        `json:"failureThreshold"`
	OpenDuration      time.Duration `json:"openDuration"`
	MaxHalfOpenProbes uint32        `json:"maxHalfOpenProbes"`
}

type CircuitRecommendation string

const (
	CircuitRecommendAttempt CircuitRecommendation = "recommend_attempt"
	CircuitRecommendProbe   CircuitRecommendation = "recommend_probe"
	CircuitRecommendBlock   CircuitRecommendation = "recommend_block"
)

type CircuitDecision struct {
	Recommendation CircuitRecommendation `json:"recommendation"`
	State          CircuitState          `json:"state"`
	Reason         string                `json:"reason"`
	Authority      AuthorityBoundary     `json:"authority"`
}

type AttemptOutcome string

const (
	AttemptSucceeded AttemptOutcome = "succeeded"
	AttemptFailed    AttemptOutcome = "failed"
)

type RecoveryAction string

const (
	RecoveryWaitWorker      RecoveryAction = "wait_for_worker"
	RecoveryReclaimLease    RecoveryAction = "reclaim_lease"
	RecoveryHoldCircuitOpen RecoveryAction = "hold_circuit_open"
	RecoveryScheduleRetry   RecoveryAction = "schedule_retry"
	RecoveryDeadLetter      RecoveryAction = "dead_letter"
	RecoveryManualReview    RecoveryAction = "manual_review"
)

type RecoveryRequest struct {
	ContractVersion   int              `json:"contractVersion"`
	Scope             Scope            `json:"scope"`
	WorkID            string           `json:"workId"`
	Now               time.Time        `json:"now"`
	Lease             *WorkLease       `json:"lease,omitempty"`
	Heartbeat         *WorkerHeartbeat `json:"heartbeat,omitempty"`
	HeartbeatMaxAge   time.Duration    `json:"heartbeatMaxAge"`
	Circuit           *CircuitState    `json:"circuit,omitempty"`
	AttemptsCompleted uint32           `json:"attemptsCompleted"`
	Failure           *Failure         `json:"failure,omitempty"`
	RetryPolicy       RetryPolicy      `json:"retryPolicy"`
}

type RecoveryDecision struct {
	Action          RecoveryAction    `json:"action"`
	NotBefore       *time.Time        `json:"notBefore,omitempty"`
	DeadLetterClass DeadLetterClass   `json:"deadLetterClass,omitempty"`
	Reason          string            `json:"reason"`
	Authority       AuthorityBoundary `json:"authority"`
}

type ControlEvent struct {
	ContractVersion int               `json:"contractVersion"`
	Scope           Scope             `json:"scope"`
	Type            string            `json:"type"`
	SubjectID       string            `json:"subjectId"`
	OccurredAt      time.Time         `json:"occurredAt"`
	Sequence        uint64            `json:"sequence"`
	PreviousHash    string            `json:"previousHash,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}
