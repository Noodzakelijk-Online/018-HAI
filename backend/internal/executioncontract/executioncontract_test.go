package executioncontract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidEnvelopeAndDeterministicDigest(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	envelope := validEnvelope(t, now)

	if err := Validate(DefaultValidationPolicy(), envelope, now); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	reordered := envelope
	reordered.ContractDigest = ""
	reordered.Action.AllowedTools = []string{"filesystem", "planner"}
	reordered.Resources = []ResourceScope{
		{
			Kind:       "document",
			Identifier: "project/report.md",
			Operations: []ResourceOperation{OperationUpdate, OperationRead},
		},
	}
	reordered.EvidenceRequirements = append(
		[]EvidenceRequirement(nil),
		envelope.EvidenceRequirements...,
	)
	reordered.SourceProvenance = append(
		[]SourceProvenance(nil),
		envelope.SourceProvenance...,
	)
	reordered.RedactedMetadata = map[string]string{
		"request_class": "code_change",
		"access_token":  RedactedValue,
	}

	before := append(
		[]ResourceOperation(nil),
		reordered.Resources[0].Operations...,
	)
	digest, err := ComputeDigest(reordered)
	if err != nil {
		t.Fatalf("ComputeDigest() error = %v", err)
	}
	if digest != envelope.ContractDigest {
		t.Fatalf("equivalent digest = %s, want %s", digest, envelope.ContractDigest)
	}
	for index := range before {
		if before[index] != reordered.Resources[0].Operations[index] {
			t.Fatalf("ComputeDigest() mutated resource operations")
		}
	}
}

func TestValidationRejectsUnsafeOrTamperedContracts(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		mutate  func(*Envelope)
		wantErr string
	}{
		{
			name: "tampered digest",
			mutate: func(value *Envelope) {
				value.Action.Purpose = "a different purpose"
			},
			wantErr: "digest does not match",
		},
		{
			name: "unredacted secret metadata",
			mutate: func(value *Envelope) {
				value.RedactedMetadata["access_token"] = "sk-proj-example"
				resetDigest(t, value)
			},
			wantErr: "must be redacted",
		},
		{
			name: "wildcard resource",
			mutate: func(value *Envelope) {
				value.Resources[0].Identifier = "*"
				resetDigest(t, value)
			},
			wantErr: "unbounded wildcard",
		},
		{
			name: "high risk without approval",
			mutate: func(value *Envelope) {
				value.Action.Risk = RiskHigh
				value.ApprovalReferences = nil
				resetDigest(t, value)
			},
			wantErr: "requires an approval",
		},
		{
			name: "approval expires before deadline",
			mutate: func(value *Envelope) {
				value.ApprovalReferences[0].ExpiresAt = now.Add(90 * time.Minute)
				resetDigest(t, value)
			},
			wantErr: "expires before",
		},
		{
			name: "invalid trace id",
			mutate: func(value *Envelope) {
				value.TraceID = strings.Repeat("0", 32)
				resetDigest(t, value)
			},
			wantErr: "all zeroes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validEnvelope(t, now)
			test.mutate(&value)
			err := Validate(DefaultValidationPolicy(), value, now)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestDeriveChildAttemptRestrictsScope(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	parent := validEnvelope(t, now)
	autonomy := 2
	childAction := parent.Action
	childAction.AllowedTools = []string{"filesystem"}
	childAction.ProhibitedActions = append(
		childAction.ProhibitedActions,
		"do not contact external parties",
	)

	child, err := DeriveChildAttempt(parent, ChildAttempt{
		AttemptID:      "22222222-2222-4222-8222-222222222222",
		IdempotencyKey: "run-1-attempt-2-write-report",
		CreatedAt:      now.Add(time.Minute),
		Deadline:       now.Add(90 * time.Minute),
		Action:         &childAction,
		Resources: []ResourceScope{
			{
				Kind:       "document",
				Identifier: "project/report.md",
				Operations: []ResourceOperation{OperationRead},
			},
		},
		AutonomyCeiling: &autonomy,
		EvidenceRequirements: []EvidenceRequirement{
			{
				ID:           "review-result",
				Kind:         EvidenceHumanReview,
				Description:  "Review the final content",
				MinimumCount: 1,
				Verifier:     "human-review",
				Required:     true,
			},
		},
		RedactedMetadata: map[string]string{"child_reason": "retry"},
	})
	if err != nil {
		t.Fatalf("DeriveChildAttempt() error = %v", err)
	}
	if child.RunID != parent.RunID ||
		child.CorrelationID != parent.CorrelationID ||
		child.TraceID != parent.TraceID {
		t.Fatalf("child lost parent execution identity")
	}
	if child.ParentAttemptID != parent.AttemptID ||
		child.ParentContractDigest != parent.ContractDigest ||
		child.AttemptNumber != 2 {
		t.Fatalf("child parent linkage is incorrect: %#v", child)
	}
	if child.AutonomyCeiling != autonomy {
		t.Fatalf("child autonomy = %d, want %d", child.AutonomyCeiling, autonomy)
	}
	if len(child.EvidenceRequirements) != 2 {
		t.Fatalf("child evidence requirements = %d, want 2", len(child.EvidenceRequirements))
	}
	if err := Validate(DefaultValidationPolicy(), child, now); err != nil {
		t.Fatalf("Validate(child) error = %v", err)
	}
}

func TestDeriveChildAttemptRejectsAuthorityExpansion(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	parent := validEnvelope(t, now)

	tests := []struct {
		name    string
		child   ChildAttempt
		wantErr string
	}{
		{
			name: "new resource",
			child: childAttempt(now, []ResourceScope{
				{
					Kind:       "document",
					Identifier: "other/secret.md",
					Operations: []ResourceOperation{OperationRead},
				},
			}),
			wantErr: "outside parent scope",
		},
		{
			name: "new operation",
			child: childAttempt(now, []ResourceScope{
				{
					Kind:       "document",
					Identifier: "project/report.md",
					Operations: []ResourceOperation{OperationDelete},
				},
			}),
			wantErr: "exceeds parent scope",
		},
		{
			name: "deadline extension",
			child: func() ChildAttempt {
				value := childAttempt(now, nil)
				value.Deadline = parent.Deadline.Add(time.Minute)
				return value
			}(),
			wantErr: "deadline cannot exceed",
		},
		{
			name: "metadata override",
			child: func() ChildAttempt {
				value := childAttempt(now, nil)
				value.RedactedMetadata = map[string]string{
					"request_class": "changed",
				}
				return value
			}(),
			wantErr: "cannot overwrite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DeriveChildAttempt(parent, test.child)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"DeriveChildAttempt() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestSafeLogProjectionExcludesSensitivePayloads(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	envelope := validEnvelope(t, now)
	envelope.RedactedMetadata["unsafe_note"] = "Bearer should-not-leak"

	projection := SafeLog(envelope)
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{
		envelope.OwnerID,
		envelope.Resources[0].Identifier,
		envelope.SourceProvenance[0].URI,
		envelope.ApprovalReferences[0].GrantedBy,
		"should-not-leak",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("safe log contains forbidden value %q: %s", forbidden, text)
		}
	}
	if projection.OwnerRef == "" ||
		projection.OwnerRef == envelope.OwnerID ||
		projection.Metadata["unsafe_note"] != RedactedValue {
		t.Fatalf("safe log redaction is incomplete: %#v", projection)
	}
}

func TestExecutionErrorTaxonomy(t *testing.T) {
	valid := ExecutionError{
		Code:      "provider_temporarily_unavailable",
		Category:  ErrorDependencyUnavailable,
		Message:   "The selected local provider is unavailable.",
		Retryable: true,
		Details: map[string]string{
			"provider": "ollama",
		},
	}
	if err := ValidateExecutionError(valid); err != nil {
		t.Fatalf("ValidateExecutionError() error = %v", err)
	}

	invalid := valid
	invalid.Category = ErrorPolicyDenied
	if err := ValidateExecutionError(invalid); err == nil {
		t.Fatalf("retryable policy denial must be rejected")
	}

	invalid = valid
	invalid.Details = map[string]string{"api_token": "secret-value"}
	if err := ValidateExecutionError(invalid); err == nil {
		t.Fatalf("unredacted error details must be rejected")
	}
}

func validEnvelope(t *testing.T, now time.Time) Envelope {
	t.Helper()
	envelope := Envelope{
		SchemaVersion:   CurrentSchemaVersion,
		OwnerID:         "owner-robert",
		RunID:           "11111111-1111-4111-8111-111111111111",
		AttemptID:       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		AttemptNumber:   1,
		CorrelationID:   "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		IdempotencyKey:  "run-1-attempt-1-write-report",
		TraceID:         "0123456789abcdef0123456789abcdef",
		CreatedAt:       now,
		Deadline:        now.Add(2 * time.Hour),
		AutonomyCeiling: 3,
		Action: ActionScope{
			Name:                "write_project_report",
			Purpose:             "Update the project report from verified evidence.",
			Mode:                ModeExecute,
			Risk:                RiskLow,
			RequiresApproval:    false,
			AllowedTools:        []string{"planner", "filesystem"},
			ProhibitedActions:   []string{"do not publish", "do not send"},
			ExpectedSideEffects: []string{"update project report"},
		},
		Resources: []ResourceScope{
			{
				Kind:       "document",
				Identifier: "project/report.md",
				Operations: []ResourceOperation{OperationRead, OperationUpdate},
			},
		},
		PolicyReferences: []PolicyReference{
			{
				ID:             "local-safe-execution",
				Version:        "v1",
				DecisionID:     "decision-001",
				DecisionDigest: strings.Repeat("a", 64),
			},
		},
		ApprovalReferences: []ApprovalReference{
			{
				ID:          "approval-001",
				GrantedBy:   "operator-robert",
				ScopeDigest: strings.Repeat("b", 64),
				GrantedAt:   now.Add(-time.Minute),
				ExpiresAt:   now.Add(3 * time.Hour),
			},
		},
		EvidenceRequirements: []EvidenceRequirement{
			{
				ID:           "test-result",
				Kind:         EvidenceTest,
				Description:  "Focused tests must pass.",
				MinimumCount: 1,
				Verifier:     "go-test",
				Required:     true,
			},
		},
		SourceProvenance: []SourceProvenance{
			{
				SourceID:      "source-001",
				SourceType:    "repository",
				SourceVersion: "commit-abc123",
				URI:           "https://example.com/repository/commit/abc123",
				ContentDigest: strings.Repeat("c", 64),
				RetrievedAt:   now.Add(-2 * time.Minute),
				Authority:     "local git repository",
			},
		},
		RedactedMetadata: map[string]string{
			"request_class": "code_change",
			"access_token":  RedactedValue,
		},
	}
	resetDigest(t, &envelope)
	return envelope
}

func resetDigest(t *testing.T, envelope *Envelope) {
	t.Helper()
	envelope.ContractDigest = ""
	digest, err := ComputeDigest(*envelope)
	if err != nil {
		t.Fatalf("ComputeDigest() error = %v", err)
	}
	envelope.ContractDigest = digest
}

func childAttempt(now time.Time, resources []ResourceScope) ChildAttempt {
	return ChildAttempt{
		AttemptID:      "22222222-2222-4222-8222-222222222222",
		IdempotencyKey: "run-1-attempt-2-write-report",
		CreatedAt:      now.Add(time.Minute),
		Resources:      resources,
	}
}
