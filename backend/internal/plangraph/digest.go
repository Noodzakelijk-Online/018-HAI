package plangraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const digestContractVersion = "hai-plan-graph-v1"

type digestEnvelope struct {
	ContractVersion string            `json:"contractVersion"`
	PlanID          string            `json:"planId"`
	OwnerIdentity   string            `json:"ownerIdentity"`
	Title           string            `json:"title"`
	Status          Status            `json:"status"`
	Revision        uint64            `json:"revision"`
	ParentRevision  uint64            `json:"parentRevision"`
	ParentDigest    string            `json:"parentDigest"`
	RequestDigest   string            `json:"requestDigest"`
	Nodes           []Node            `json:"nodes"`
	Edges           []Edge            `json:"edges"`
	Repair          *RepairProvenance `json:"repair,omitempty"`
	CreatedBy       string            `json:"createdBy"`
	CreatedAt       string            `json:"createdAt"`
	AcceptedAt      string            `json:"acceptedAt,omitempty"`
}

func computeDigest(plan Plan) (string, error) {
	plan = normalizePlan(plan)
	payload, err := json.Marshal(digestEnvelope{
		ContractVersion: digestContractVersion,
		PlanID:          plan.ID.String(),
		OwnerIdentity:   plan.OwnerIdentity,
		Title:           plan.Title,
		Status:          plan.Status,
		Revision:        plan.Revision,
		ParentRevision:  plan.ParentRevision,
		ParentDigest:    plan.ParentDigest,
		RequestDigest:   plan.RequestDigest,
		Nodes:           plan.Nodes,
		Edges:           plan.Edges,
		Repair:          plan.Repair,
		CreatedBy:       plan.CreatedBy,
		CreatedAt:       plan.CreatedAt.Format(time.RFC3339Nano),
		AcceptedAt:      formatDigestTime(plan.AcceptedAt),
	})
	if err != nil {
		return "", fmt.Errorf("encode canonical plan graph: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func formatDigestTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Round(0).Format(time.RFC3339Nano)
}

func computeRequestDigest(title string, nodes []Node, edges []Edge, reason, trigger string) (string, error) {
	canonical := normalizePlan(Plan{
		Title: title, Nodes: nodes, Edges: edges,
		Repair: &RepairProvenance{Reason: reason, Trigger: trigger},
	})
	payload, err := json.Marshal(struct {
		ContractVersion string `json:"contractVersion"`
		Title           string `json:"title"`
		Nodes           []Node `json:"nodes"`
		Edges           []Edge `json:"edges"`
		Reason          string `json:"reason,omitempty"`
		Trigger         string `json:"trigger,omitempty"`
	}{digestContractVersion, canonical.Title, canonical.Nodes, canonical.Edges, canonical.Repair.Reason, canonical.Repair.Trigger})
	if err != nil {
		return "", fmt.Errorf("encode canonical plan request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
