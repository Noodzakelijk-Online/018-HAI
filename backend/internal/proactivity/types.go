// Package proactivity ranks owner-scoped open loops and recommends how much
// attention they deserve. It is a pure policy engine: it never sends a
// notification, executes an action, or grants authority.
package proactivity

import "time"

const ContractVersion = 1

const (
	MaxSignals         = 256
	MaxHistoryEntries  = 2048
	MaxEvidencePerItem = 16
)

type Outcome string

const (
	OutcomeSuppress      Outcome = "suppress"
	OutcomeAmbient       Outcome = "ambient"
	OutcomeDailyBrief    Outcome = "daily_brief"
	OutcomeNotify        Outcome = "notify"
	OutcomeRequireReview Outcome = "require_review"
)

type SignalStatus string

const (
	StatusOpen     SignalStatus = "open"
	StatusResolved SignalStatus = "resolved"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Channel string

const (
	ChannelInApp   Channel = "in_app"
	ChannelDesktop Channel = "desktop"
	ChannelEmail   Channel = "email"
	ChannelSMS     Channel = "sms"
	ChannelWebhook Channel = "webhook"
)

func (c Channel) Local() bool {
	return c == ChannelInApp || c == ChannelDesktop
}

type QuietHours struct {
	Enabled     bool   `json:"enabled"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	TimeZone    string `json:"timeZone"`
}

type ChannelPreference struct {
	Channel Channel `json:"channel"`
	Enabled bool    `json:"enabled"`
	Order   int     `json:"order"`
}

type AttentionBudget struct {
	MaxInterruptionsPerDay int `json:"maxInterruptionsPerDay"`
}

type Preferences struct {
	ContractVersion       int                 `json:"contractVersion"`
	OwnerIdentity         string              `json:"ownerIdentity"`
	TimeZone              string              `json:"timeZone"`
	QuietHours            QuietHours          `json:"quietHours"`
	MinimumConfidence     float64             `json:"minimumConfidence"`
	AmbientThreshold      float64             `json:"ambientThreshold"`
	DailyBriefThreshold   float64             `json:"dailyBriefThreshold"`
	NotifyThreshold       float64             `json:"notifyThreshold"`
	ReviewThreshold       float64             `json:"reviewThreshold"`
	Cooldown              time.Duration       `json:"cooldown"`
	AttentionBudget       AttentionBudget     `json:"attentionBudget"`
	Channels              []ChannelPreference `json:"channels"`
	AllowExternalChannels bool                `json:"allowExternalChannels"`
}

func DefaultPreferences(owner string) Preferences {
	return Preferences{
		ContractVersion:     ContractVersion,
		OwnerIdentity:       owner,
		TimeZone:            "UTC",
		MinimumConfidence:   0.55,
		AmbientThreshold:    0.30,
		DailyBriefThreshold: 0.50,
		NotifyThreshold:     0.72,
		ReviewThreshold:     0.88,
		Cooldown:            12 * time.Hour,
		AttentionBudget: AttentionBudget{
			MaxInterruptionsPerDay: 4,
		},
		Channels: []ChannelPreference{
			{Channel: ChannelInApp, Enabled: true, Order: 0},
			{Channel: ChannelDesktop, Enabled: true, Order: 1},
		},
	}
}

type EvidenceReference struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Digest     string    `json:"digest"`
	ObservedAt time.Time `json:"observedAt"`
}

type OpenLoopSignal struct {
	ContractVersion     int                 `json:"contractVersion"`
	ID                  string              `json:"id"`
	OwnerIdentity       string              `json:"ownerIdentity"`
	OpenLoopKey         string              `json:"openLoopKey"`
	Title               string              `json:"title"`
	Summary             string              `json:"summary"`
	Status              SignalStatus        `json:"status"`
	Risk                RiskLevel           `json:"risk"`
	ObservedAt          time.Time           `json:"observedAt"`
	LastActivityAt      time.Time           `json:"lastActivityAt"`
	Deadline            *time.Time          `json:"deadline,omitempty"`
	StaleAfter          time.Duration       `json:"staleAfter"`
	Impact              float64             `json:"impact"`
	Urgency             float64             `json:"urgency"`
	Confidence          float64             `json:"confidence"`
	Sensitive           bool                `json:"sensitive"`
	HumanReviewRequired bool                `json:"humanReviewRequired"`
	Evidence            []EvidenceReference `json:"evidence,omitempty"`
}

type DecisionHistory struct {
	ContractVersion int       `json:"contractVersion"`
	OwnerIdentity   string    `json:"ownerIdentity"`
	OpenLoopKey     string    `json:"openLoopKey"`
	SignalDigest    string    `json:"signalDigest"`
	Outcome         Outcome   `json:"outcome"`
	DecidedAt       time.Time `json:"decidedAt"`
}

type EvaluationRequest struct {
	ContractVersion int                `json:"contractVersion"`
	OwnerIdentity   string             `json:"ownerIdentity"`
	Now             time.Time          `json:"now"`
	Preferences     Preferences        `json:"preferences"`
	Signals         []OpenLoopSignal   `json:"signals"`
	History         []DecisionHistory  `json:"history,omitempty"`
	Controls        []AttentionControl `json:"controls,omitempty"`
}

type ScoreComponent struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Explanation  string  `json:"explanation"`
}

type Decision struct {
	ContractVersion     int              `json:"contractVersion"`
	OwnerIdentity       string           `json:"ownerIdentity"`
	SignalID            string           `json:"signalId"`
	OpenLoopKey         string           `json:"openLoopKey"`
	SignalDigest        string           `json:"signalDigest"`
	Title               string           `json:"title"`
	Summary             string           `json:"summary"`
	Outcome             Outcome          `json:"outcome"`
	Score               float64          `json:"score"`
	Components          []ScoreComponent `json:"components"`
	Reasons             []string         `json:"reasons"`
	RecommendedChannels []Channel        `json:"recommendedChannels,omitempty"`
	NextEligibleAt      *time.Time       `json:"nextEligibleAt,omitempty"`
	BudgetCost          int              `json:"budgetCost"`
	ExecutionAuthorized bool             `json:"executionAuthorized"`
	DeliveryAuthorized  bool             `json:"deliveryAuthorized"`
	AuthorityGranted    bool             `json:"authorityGranted"`
	DecidedAt           time.Time        `json:"decidedAt"`
}

type EvaluationResult struct {
	ContractVersion        int        `json:"contractVersion"`
	OwnerIdentity          string     `json:"ownerIdentity"`
	DecidedAt              time.Time  `json:"decidedAt"`
	TimeZone               string     `json:"timeZone"`
	InterruptionsUsed      int        `json:"interruptionsUsed"`
	InterruptionsRemaining int        `json:"interruptionsRemaining"`
	Decisions              []Decision `json:"decisions"`
}
