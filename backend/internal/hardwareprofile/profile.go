package hardwareprofile

import (
	"runtime"
	"strings"
	"time"
)

// HardwareProfile is the detected/declared local hardware profile (§18). Fields
// that cannot be truthfully detected are left unknown/empty rather than guessed.
type HardwareProfile struct {
	ID                  string              `json:"id"`
	OwnerUserID         string              `json:"ownerUserId"`
	WorkspaceID         string              `json:"workspaceId"`
	OperatingSystem     string              `json:"operatingSystem"`
	WindowsVersion      string              `json:"windowsVersion"`
	BuildNumber         string              `json:"buildNumber"`
	PowerMode           string              `json:"powerMode"`
	BatteryStatus       string              `json:"batteryStatus"`
	CPUModel            string              `json:"cpuModel"`
	CPUCores            int                 `json:"cpuCores"`
	RAMTotal            uint64              `json:"ramTotal"`
	GPUVendor           string              `json:"gpuVendor"`
	GPUModel            string              `json:"gpuModel"`
	GPUVram             uint64              `json:"gpuVram"`
	NPUVendor           string              `json:"npuVendor"`
	NPUModel            string              `json:"npuModel"`
	NPUTopsDeclared     float64             `json:"npuTopsDeclared"`
	DriverVersions      string              `json:"driverVersions"`
	DockerAvailable     bool                `json:"dockerAvailable"`
	DockerDaemonRunning bool                `json:"dockerDaemonRunning"`
	WSLAvailable        bool                `json:"wslAvailable"`
	ExecutionProviders  []ExecutionProvider `json:"executionProviders"`
	LocalModelRuntimes  []string            `json:"localModelRuntimes"`
	LastDetectedAt      *time.Time          `json:"lastDetectedAt,omitempty"`
	CreatedAt           time.Time           `json:"createdAt"`
	UpdatedAt           time.Time           `json:"updatedAt"`
}

// Detect performs truthful, dependency-free detection. It fills only what the
// Go runtime can honestly report; GPU/NPU/Windows specifics are left unknown
// unless the OS is actually Windows and are never fabricated (§18).
func Detect(ownerUserID, workspaceID string, now time.Time) HardwareProfile {
	os := runtime.GOOS
	p := HardwareProfile{
		OwnerUserID:     ownerUserID,
		WorkspaceID:     workspaceID,
		OperatingSystem: os,
		CPUCores:        runtime.NumCPU(),
		PowerMode:       "unknown",
		BatteryStatus:   "unknown",
		GPUVendor:       "unknown",
		NPUVendor:       "unknown",
		LastDetectedAt:  &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Base execution provider always truthfully available: ONNX CPU is a safe,
	// universally-available label; everything else requires real detection.
	p.ExecutionProviders = []ExecutionProvider{EPOnnxCPU}

	if os != "windows" {
		// Do not claim Windows ML / NPU / DirectML on non-Windows (§18).
		p.WindowsVersion = ""
		p.BuildNumber = ""
	}
	return p
}

// SelectServingStack chooses a serving stack from the profile using the §18
// selection rules. It prefers native Windows ONNX, then WSL2+CUDA, then
// TensorRT-RTX, then llama.cpp for low-VRAM, then Ollama/LM Studio; DirectML is
// treated as legacy fallback; cloud only as a last resort. Unknown hardware
// yields onnx_runtime_cpu — never an invented accelerator path.
func (p HardwareProfile) SelectServingStack() ServingStack {
	has := func(ep ExecutionProvider) bool {
		for _, x := range p.ExecutionProviders {
			if x == ep {
				return true
			}
		}
		return false
	}
	windows := strings.EqualFold(p.OperatingSystem, "windows")

	switch {
	case windows && has(EPWindowsML):
		return StackWindowsMLOnnx
	case has(EPWSL2CUDA):
		return StackWSL2CUDA
	case has(EPOnnxTensorRTRTX):
		return StackOnnxTensorRTRTX
	case has(EPOnnxQNN):
		return StackOnnxQNN
	case has(EPOnnxOpenVINO):
		return StackOnnxOpenVINO
	case has(EPLlamaCPP):
		return StackLlamaCPPGGUF
	case has(EPOllama):
		return StackOllama
	case has(EPLMStudio):
		return StackLMStudio
	case has(EPOnnxDirectML):
		return StackDirectMLLegacy // legacy fallback, not first choice
	case has(EPOnnxCPU):
		return StackOnnxCPU
	default:
		return StackUnknown
	}
}
