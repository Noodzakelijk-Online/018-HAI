package models

import (
	"github.com/google/uuid"
	"time"
)

type AutomationHealthEvent struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID        uuid.UUID `gorm:"type:uuid;index" json:"automationId"`
	Status              string    `gorm:"type:varchar(30);index" json:"status"`
	CheckType           string    `gorm:"type:varchar(50)" json:"checkType"`
	Target              string    `gorm:"type:varchar(1024)" json:"target,omitempty"`
	LatencyMs           int64     `gorm:"default:0" json:"latencyMs"`
	FailureReason       string    `gorm:"type:text" json:"failureReason,omitempty"`
	ConsecutiveFailures int       `gorm:"default:0" json:"consecutiveFailures"`
	CheckedAt           time.Time `gorm:"index" json:"checkedAt"`
}

type AutomationDependency struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID uuid.UUID  `gorm:"type:uuid;index" json:"automationId"`
	Name         string     `gorm:"type:varchar(255);not null" json:"name"`
	Kind         string     `gorm:"type:varchar(50);not null" json:"kind"`
	Target       string     `gorm:"type:varchar(1024)" json:"target,omitempty"`
	Required     bool       `gorm:"default:true" json:"required"`
	Status       string     `gorm:"type:varchar(30);default:'unknown'" json:"status"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	Notes        string     `gorm:"type:text" json:"notes,omitempty"`
}

type AutomationRouteCheck struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID   uuid.UUID  `gorm:"type:uuid;index" json:"automationId"`
	ExpectedRoute  string     `gorm:"type:varchar(255)" json:"expectedRoute"`
	ExpectedHost   string     `gorm:"type:varchar(255)" json:"expectedHost,omitempty"`
	ExpectedPort   int        `json:"expectedPort,omitempty"`
	ExpectedStatus int        `gorm:"default:200" json:"expectedStatus,omitempty"`
	Status         string     `gorm:"type:varchar(30);default:'unknown'" json:"status"`
	FailureReason  string     `gorm:"type:text" json:"failureReason,omitempty"`
	LastCheckedAt  *time.Time `json:"lastCheckedAt,omitempty"`
}

type AutomationAlert struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID   uuid.UUID  `gorm:"type:uuid;index" json:"automationId"`
	Severity       string     `gorm:"type:varchar(30);index" json:"severity"`
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Message        string     `gorm:"type:text" json:"message"`
	Status         string     `gorm:"type:varchar(30);default:'open';index" json:"status"`
	FirstSeenAt    time.Time  `gorm:"index" json:"firstSeenAt"`
	LastSeenAt     time.Time  `gorm:"index" json:"lastSeenAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
}

type AutomationIncident struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID   uuid.UUID  `gorm:"type:uuid;index" json:"automationId"`
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Severity       string     `gorm:"type:varchar(30);index" json:"severity"`
	Status         string     `gorm:"type:varchar(30);default:'open';index" json:"status"`
	StartedAt      time.Time  `gorm:"index" json:"startedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	RootCause      string     `gorm:"type:text" json:"rootCause,omitempty"`
	ResolutionNote string     `gorm:"type:text" json:"resolutionNote,omitempty"`
}

type AutomationSLO struct {
	ID                    uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	AutomationID          uuid.UUID `gorm:"type:uuid;index" json:"automationId"`
	AvailabilityTargetPct float64   `gorm:"default:99" json:"availabilityTargetPct"`
	MaxLatencyMs          int64     `gorm:"default:5000" json:"maxLatencyMs"`
	MaxConsecutiveFailures int      `gorm:"default:3" json:"maxConsecutiveFailures"`
	MonitoringWindowHours int       `gorm:"default:24" json:"monitoringWindowHours"`
	Notes                 string    `gorm:"type:text" json:"notes,omitempty"`
}
