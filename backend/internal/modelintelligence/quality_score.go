package modelintelligence

// QualityInputs are the raw signals for the efficiency score (§18). True score =
// useful_verified_work / total_resource_cost. Raw tokens/sec is not enough.
type QualityInputs struct {
	VerifiedCompletions    int `json:"verifiedCompletions"`
	SourceGroundedOutputs  int `json:"sourceGroundedOutputs"`
	HighRiskBlockedCorrect int `json:"highRiskBlockedCorrectly"`
	ApprovalsAvoidedSafely int `json:"approvalsAvoidedSafely"`

	TokensUsed            int     `json:"tokensUsed"`
	LatencyMs             int64   `json:"latencyMs"`
	PaidCostEUR           float64 `json:"paidCostEur"`
	PrivacyRiskEvents     int     `json:"privacyRiskEvents"`
	VerificationFailures  int     `json:"verificationFailures"`
	HumanRepairs          int     `json:"humanRepairs"`
	UnnecessaryApprovals  int     `json:"unnecessaryApprovals"`
	FailedAutonomousTries int     `json:"failedAutonomousAttempts"`
}

// QualityScore is the computed efficiency score plus its breakdown.
type QualityScore struct {
	UsefulWork    float64 `json:"usefulWork"`
	ResourceCost  float64 `json:"resourceCost"`
	Score         float64 `json:"score"`
	VerifiedPer1K float64 `json:"verifiedCompletionsPer1kTokens"`
}

// ComputeQualityScore computes useful_verified_work / total_resource_cost (§18).
// The cost floor is 1 so the score is always finite.
func ComputeQualityScore(in QualityInputs) QualityScore {
	useful := float64(in.VerifiedCompletions)*3 +
		float64(in.SourceGroundedOutputs)*2 +
		float64(in.HighRiskBlockedCorrect)*2 +
		float64(in.ApprovalsAvoidedSafely)*1

	cost := 1.0 +
		float64(in.TokensUsed)/1000.0 +
		float64(in.LatencyMs)/1000.0 +
		in.PaidCostEUR*10 +
		float64(in.PrivacyRiskEvents)*3 +
		float64(in.VerificationFailures)*3 +
		float64(in.HumanRepairs)*2 +
		float64(in.UnnecessaryApprovals)*1 +
		float64(in.FailedAutonomousTries)*2

	score := useful / cost
	var per1k float64
	if in.TokensUsed > 0 {
		per1k = float64(in.VerifiedCompletions) / (float64(in.TokensUsed) / 1000.0)
	}
	return QualityScore{UsefulWork: useful, ResourceCost: cost, Score: score, VerifiedPer1K: per1k}
}
