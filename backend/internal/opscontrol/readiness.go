package opscontrol

// GateStatus is a readiness gate outcome (§30 manual verification checklist).
type GateStatus string

const (
	GatePass          GateStatus = "pass"
	GateWarn          GateStatus = "warn"
	GateFail          GateStatus = "fail"
	GatePending       GateStatus = "pending"        // requires target-machine verification
	GateNotApplicable GateStatus = "not_applicable" // e.g. Windows gate on non-Windows
)

// ReadinessGate is one checklist item with evidence + remediation.
type ReadinessGate struct {
	Name        string     `json:"name"`
	Status      GateStatus `json:"status"`
	Evidence    string     `json:"evidence"`
	Remediation string     `json:"remediation,omitempty"`
}

// Readiness is the Windows-runtime readiness roll-up (§30/§31). It is honest:
// Windows-specific capabilities are pending until verified on the target
// machine, and always-on operation is not claimed unless the process survives
// restart (which the persisted emergency-stop + recovery prove).
type Readiness struct {
	OperatingSystem     string             `json:"operatingSystem"`
	IsWindows           bool               `json:"isWindows"`
	OverallReady        bool               `json:"overallReady"`
	TargetVerifyPending bool               `json:"targetMachineVerificationPending"`
	Mode                string             `json:"backgroundMode"`
	Emergency           EmergencyStopState `json:"emergencyStop"`
	Docker              DockerStatus       `json:"docker"`
	Gates               []ReadinessGate    `json:"gates"`
}
