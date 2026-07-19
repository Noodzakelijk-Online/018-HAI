// Package planningoptimizer integrates a bounded local Google OR-Tools solver
// into HAI's planning back office. It returns an explicit proposal only: no
// workflow, calendar, task, or external system is changed by this package.
package planningoptimizer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

const (
	enabledEnv  = "HAI_PLANNING_OPTIMIZER_ENABLED"
	baseURLEnv  = "HAI_PLANNING_OPTIMIZER_BASE_URL"
	timeoutEnv  = "HAI_PLANNING_OPTIMIZER_TIMEOUT_SECONDS"
	maxJobs     = 100
	maxBodySize = 1 << 20
)

var (
	ErrNotConfigured = errors.New("planning optimizer is not configured")
	ErrUnavailable   = errors.New("planning optimizer is unavailable")
)

// Job is an opaque scheduling input. IDs must not carry source text; callers
// join the returned IDs to their own owner-scoped records after review.
type Job struct {
	ID               string `json:"id"`
	DurationMinutes  int    `json:"durationMinutes"`
	Priority         int    `json:"priority"`
	EarliestMinute   *int   `json:"earliestMinute,omitempty"`
	LatestEndMinute  *int   `json:"latestEndMinute,omitempty"`
	FixedStartMinute *int   `json:"fixedStartMinute,omitempty"`
}

// Request is one bounded, one-lane schedule proposal problem.
type Request struct {
	DayStartMinute int   `json:"dayStartMinute"`
	DayEndMinute   int   `json:"dayEndMinute"`
	Jobs           []Job `json:"jobs"`
}

type ScheduledJob struct {
	ID          string `json:"id"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Priority    int    `json:"priority"`
}

// Proposal is the solver result HAI shows to a user. Its assumptions make the
// limitations explicit rather than letting a schedule look authoritative.
type Proposal struct {
	Status         string         `json:"status"`
	Solver         string         `json:"solver"`
	Scheduled      []ScheduledJob `json:"scheduled"`
	Deferred       []string       `json:"deferred"`
	ObjectiveValue *int64         `json:"objectiveValue,omitempty"`
	Assumptions    []string       `json:"assumptions"`
}

// Run is the durable, owner-scoped proposal audit returned by the back office.
type Run struct {
	ID            uuid.UUID `json:"id"`
	RequestDigest string    `json:"requestDigest"`
	Status        string    `json:"status"`
	Solver        string    `json:"solver,omitempty"`
	Summary       string    `json:"summary"`
	Result        Proposal  `json:"result"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Status struct {
	Enabled     bool   `json:"enabled"`
	Configured  bool   `json:"configured"`
	BaseURL     string `json:"baseUrl,omitempty"`
	ConfigError string `json:"configError,omitempty"`
	Scope       string `json:"scope"`
}

type config struct {
	enabled bool
	baseURL string
	timeout time.Duration
}

type service struct {
	config    config
	configErr string
	client    *http.Client
	repo      Repository
	now       func() time.Time
}

func NewService(repo Repository, enabled bool, baseURL string, timeout time.Duration) *service {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	svc := &service{
		config: config{enabled: enabled, baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), timeout: timeout},
		client: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		},
		repo: repo,
		now:  time.Now,
	}
	if enabled {
		svc.configErr = validateLocalSolverURL(svc.config.baseURL)
	}
	return svc
}

func DefaultService() *service {
	timeout := 5 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if parsed, err := time.ParseDuration(raw + "s"); err == nil {
			timeout = parsed
		}
	}
	return NewService(
		DefaultRepository(),
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		strings.TrimSpace(os.Getenv(baseURLEnv)),
		timeout,
	)
}

func (s *service) Status() Status {
	return Status{
		Enabled:     s.config.enabled,
		Configured:  s.config.enabled && s.configErr == "",
		BaseURL:     s.config.baseURL,
		ConfigError: s.configErr,
		Scope:       "Local OR-Tools scheduling proposals only. HAI never applies a proposal to workflows, calendars, or external systems.",
	}
}

func (s *service) Propose(ctx context.Context, ownerIdentity string, request Request) (*Run, error) {
	if strings.TrimSpace(ownerIdentity) == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	digest, err := digestRequest(request)
	if err != nil {
		return nil, err
	}
	if !s.config.enabled || s.configErr != "" {
		run, persistErr := s.persist(ownerIdentity, digest, Proposal{Status: "not_configured"}, "planning optimizer is not configured; no solver request was made")
		if persistErr != nil {
			return nil, persistErr
		}
		return run, ErrNotConfigured
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.baseURL+"/v1/schedule", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "HAI-PlanningOptimizer/1.0")
	response, err := s.client.Do(httpRequest)
	if err != nil {
		_, _ = s.persist(ownerIdentity, digest, Proposal{Status: "unavailable"}, "local OR-Tools service was unavailable")
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = s.persist(ownerIdentity, digest, Proposal{Status: "failed"}, fmt.Sprintf("local OR-Tools service returned HTTP %d", response.StatusCode))
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBodySize+1))
	if err != nil || len(data) > maxBodySize {
		_, _ = s.persist(ownerIdentity, digest, Proposal{Status: "failed"}, "local OR-Tools service returned an unusable response")
		return nil, ErrUnavailable
	}
	var proposal Proposal
	if err := json.Unmarshal(data, &proposal); err != nil || !validProposal(proposal, request) {
		_, _ = s.persist(ownerIdentity, digest, Proposal{Status: "failed"}, "local OR-Tools service returned an invalid proposal")
		return nil, ErrUnavailable
	}
	return s.persist(ownerIdentity, digest, proposal, summary(proposal))
}

func (s *service) Runs(ownerIdentity string, limit int) ([]Run, error) {
	records, err := s.repo.List(ownerIdentity, limit)
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(records))
	for _, record := range records {
		var proposal Proposal
		if err := json.Unmarshal([]byte(record.ResultJSON), &proposal); err != nil {
			proposal = Proposal{Status: record.Status}
		}
		runs = append(runs, runFromModel(record, proposal))
	}
	return runs, nil
}

func (s *service) persist(owner, digest string, proposal Proposal, message string) (*Run, error) {
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return nil, err
	}
	record := &models.OptimizationProposalRun{
		OwnerIdentity: owner,
		RequestDigest: digest,
		Status:        proposal.Status,
		Solver:        truncate(proposal.Solver, 160),
		Summary:       truncate(message, 500),
		ResultJSON:    string(encoded),
		CreatedAt:     s.now().UTC(),
	}
	stored, err := s.repo.Create(record)
	if err != nil {
		return nil, err
	}
	run := runFromModel(*stored, proposal)
	return &run, nil
}

func runFromModel(record models.OptimizationProposalRun, proposal Proposal) Run {
	return Run{
		ID:            record.ID,
		RequestDigest: record.RequestDigest,
		Status:        record.Status,
		Solver:        record.Solver,
		Summary:       record.Summary,
		Result:        proposal,
		CreatedAt:     record.CreatedAt,
	}
}

func validateRequest(request Request) error {
	if request.DayStartMinute < 0 || request.DayEndMinute > 24*60 || request.DayEndMinute <= request.DayStartMinute {
		return fmt.Errorf("day bounds must be valid minute values")
	}
	if len(request.Jobs) == 0 || len(request.Jobs) > maxJobs {
		return fmt.Errorf("jobs must contain 1 to %d items", maxJobs)
	}
	seen := map[string]bool{}
	for _, job := range request.Jobs {
		if !validID(job.ID) || seen[job.ID] {
			return fmt.Errorf("job ids must be unique opaque identifiers")
		}
		seen[job.ID] = true
		if job.DurationMinutes < 1 || job.DurationMinutes > 24*60 || job.Priority < 1 || job.Priority > 100 {
			return fmt.Errorf("job %s has invalid duration or priority", job.ID)
		}
		earliest := request.DayStartMinute
		if job.EarliestMinute != nil {
			earliest = *job.EarliestMinute
		}
		latestEnd := request.DayEndMinute
		if job.LatestEndMinute != nil {
			latestEnd = *job.LatestEndMinute
		}
		if earliest < request.DayStartMinute || latestEnd > request.DayEndMinute || earliest+job.DurationMinutes > latestEnd {
			return fmt.Errorf("job %s cannot fit inside its time window", job.ID)
		}
		if job.FixedStartMinute != nil && (*job.FixedStartMinute < earliest || *job.FixedStartMinute+job.DurationMinutes > latestEnd) {
			return fmt.Errorf("job %s has an invalid fixed start", job.ID)
		}
	}
	return nil
}

func validProposal(proposal Proposal, request Request) bool {
	if !validSolverStatus(proposal.Status) || len(proposal.Solver) == 0 || len(proposal.Solver) > 160 || len(proposal.Assumptions) == 0 || len(proposal.Assumptions) > 10 {
		return false
	}
	if len(proposal.Scheduled)+len(proposal.Deferred) != len(request.Jobs) {
		return false
	}
	inputs := make(map[string]Job, len(request.Jobs))
	for _, job := range request.Jobs {
		inputs[job.ID] = job
	}
	seen := make(map[string]bool, len(request.Jobs))
	lastEnd := -1
	for _, job := range proposal.Scheduled {
		input, ok := inputs[job.ID]
		if !ok || seen[job.ID] || !validID(job.ID) || job.EndMinute-job.StartMinute != input.DurationMinutes || job.Priority != input.Priority {
			return false
		}
		earliest, latestEnd := inputWindow(request, input)
		if job.StartMinute < earliest || job.EndMinute > latestEnd || job.StartMinute < lastEnd {
			return false
		}
		seen[job.ID] = true
		lastEnd = job.EndMinute
	}
	for _, id := range proposal.Deferred {
		if _, ok := inputs[id]; !ok || seen[id] || !validID(id) {
			return false
		}
		seen[id] = true
	}
	for _, assumption := range proposal.Assumptions {
		if len(assumption) == 0 || len(assumption) > 300 {
			return false
		}
	}
	return true
}

func inputWindow(request Request, job Job) (int, int) {
	earliest, latestEnd := request.DayStartMinute, request.DayEndMinute
	if job.EarliestMinute != nil {
		earliest = *job.EarliestMinute
	}
	if job.LatestEndMinute != nil {
		latestEnd = *job.LatestEndMinute
	}
	return earliest, latestEnd
}

func validSolverStatus(status string) bool {
	switch status {
	case "optimal", "feasible", "infeasible", "model_invalid", "unknown":
		return true
	default:
		return false
	}
}

func digestRequest(request Request) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func summary(proposal Proposal) string {
	return fmt.Sprintf("OR-Tools returned %s: %d scheduled and %d deferred; the result is awaiting human review", proposal.Status, len(proposal.Scheduled), len(proposal.Deferred))
}

func validateLocalSolverURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return baseURLEnv + " must be a valid local http/https URL"
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" {
		return baseURLEnv + " must be a credential-free local http/https URL without query or fragment"
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || host == "host.docker.internal" || host == "ortools-solver" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return baseURLEnv + " may only target localhost, loopback IPs, host.docker.internal, or ortools-solver"
}

func validID(value string) bool {
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
