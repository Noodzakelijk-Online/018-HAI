package executionauth

import "testing"

func TestTaskEngineSystemWorkloadProfilesMatchExactOperationContract(t *testing.T) {
	tests := []struct {
		name     string
		request  Request
		policyID string
	}{
		{
			name: "autonomous read",
			request: Request{ActorIdentity: "hai-task-engine", Action: "automation.api.read", Stage: StageDataAccess,
				ResourceType: "automation", ToolID: "automation-api-client", RequiredAuthority: 8,
				RequestedAutonomy: 8, Risk: RiskLow, Reversible: true},
			policyID: "task-automation-api-read-autonomous-v1",
		},
		{
			name: "case approved read",
			request: Request{ActorIdentity: "hai-task-engine", Action: "automation.api.read", Stage: StageDataAccess,
				ResourceType: "automation", ToolID: "automation-api-client", RequiredAuthority: 6,
				RequestedAutonomy: 6, Risk: RiskLow, Reversible: true},
			policyID: "task-automation-api-read-approved-v1",
		},
		{
			name: "case approved script",
			request: Request{ActorIdentity: "hai-task-engine", Action: "automation.script.execute", Stage: StageExecution,
				ResourceType: "automation", ToolID: "automation-script-runner", RequiredAuthority: 6,
				RequestedAutonomy: 6, Risk: RiskHigh, Reversible: false},
			policyID: "task-automation-script-v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence, err := evaluateSystemWorkload(test.request)
			if err != nil {
				t.Fatalf("evaluateSystemWorkload: %v", err)
			}
			if !evidence.Matched || evidence.PolicyID != test.policyID {
				t.Fatalf("evidence = %#v, want matched policy %q", evidence, test.policyID)
			}
		})
	}
}

func TestTaskEngineSystemWorkloadRejectsUnknownEffectAndSelfReclassification(t *testing.T) {
	unknown := Request{ActorIdentity: "hai-task-engine", Action: "automation.unknown", Stage: StageExecution,
		ResourceType: "automation", ToolID: "automation-api-client", RequiredAuthority: 6,
		RequestedAutonomy: 6, Risk: RiskLow, Reversible: true}
	if _, err := evaluateSystemWorkload(unknown); err == nil || err.Error() != "system workload effect does not match its registered operation contract" {
		t.Fatalf("unknown effect error = %v", err)
	}

	reclassified := Request{ActorIdentity: "hai-task-engine", Action: "automation.api.read", Stage: StageDataAccess,
		ResourceType: "automation", ToolID: "automation-api-client", RequiredAuthority: 6,
		RequestedAutonomy: 7, Risk: RiskLow, Reversible: true}
	if _, err := evaluateSystemWorkload(reclassified); err == nil || err.Error() != "system workload classification differs from its server-owned policy" {
		t.Fatalf("reclassified effect error = %v", err)
	}
}
