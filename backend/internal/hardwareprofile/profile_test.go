package hardwareprofile

import (
	"runtime"
	"testing"
	"time"
)

func TestDetectIsTruthfulOnNonWindows(t *testing.T) {
	now := time.Now()
	p := Detect("user-1", "local", now)
	if p.OperatingSystem != runtime.GOOS {
		t.Fatalf("operating system must be truthful, got %q", p.OperatingSystem)
	}
	if p.CPUCores != runtime.NumCPU() {
		t.Fatalf("cpu cores must be truthful")
	}
	if runtime.GOOS != "windows" {
		// Must not claim Windows ML / NPU / GPU on non-Windows.
		for _, ep := range p.ExecutionProviders {
			if ep == EPWindowsML || ep == EPOnnxDirectML || ep == EPOnnxQNN {
				t.Fatalf("must not claim Windows/NPU execution provider on non-Windows: %s", ep)
			}
		}
		if p.WindowsVersion != "" {
			t.Fatalf("must not report a Windows version on non-Windows")
		}
		if p.GPUVendor != "unknown" || p.NPUVendor != "unknown" {
			t.Fatalf("GPU/NPU must be unknown without detection, got %q/%q", p.GPUVendor, p.NPUVendor)
		}
	}
}

func TestServingStackSelection(t *testing.T) {
	base := Detect("u", "local", time.Now())
	// CPU-only default -> onnx_runtime_cpu, never an invented accelerator.
	if got := base.SelectServingStack(); got != StackOnnxCPU {
		t.Fatalf("cpu-only profile must select onnx_runtime_cpu, got %s", got)
	}

	// Operator-declared llama.cpp -> GGUF path.
	base.ExecutionProviders = []ExecutionProvider{EPOnnxCPU, EPLlamaCPP}
	if got := base.SelectServingStack(); got != StackLlamaCPPGGUF {
		t.Fatalf("llama.cpp profile must select llama_cpp_gguf, got %s", got)
	}

	base.ExecutionProviders = []ExecutionProvider{EPOnnxCPU, EPLocalAI}
	if got := base.SelectServingStack(); got != StackLocalAI {
		t.Fatalf("LocalAI profile must select localai, got %s", got)
	}

	// DirectML is only a legacy fallback, never chosen over llama.cpp/ollama.
	base.ExecutionProviders = []ExecutionProvider{EPOnnxDirectML}
	if got := base.SelectServingStack(); got != StackDirectMLLegacy {
		t.Fatalf("directml-only profile must select the legacy fallback, got %s", got)
	}
}

func TestPowerPolicyDefersOnBattery(t *testing.T) {
	p := DefaultPowerPolicy()
	if !p.AllowsHeavyWorkNow("plugged_in") {
		t.Fatalf("balanced policy must allow heavy work on AC")
	}
	if p.AllowsHeavyWorkNow("on_battery") {
		t.Fatalf("balanced policy must defer heavy work on battery")
	}
}

func TestServicePatchOverridesAndPower(t *testing.T) {
	s := NewService("u", "local")
	vendor := "NVIDIA"
	eps := []ExecutionProvider{EPOnnxCPU, EPWSL2CUDA}
	got := s.Patch(HardwareProfilePatch{GPUVendor: &vendor, ExecutionProviders: &eps})
	if got.GPUVendor != "NVIDIA" {
		t.Fatalf("operator GPU override must apply")
	}
	if s.SelectedServingStack() != StackWSL2CUDA {
		t.Fatalf("with WSL2+CUDA declared, serving stack must be wsl2_cuda, got %s", s.SelectedServingStack())
	}
	pol := s.SetPower(PowerPolicy{Mode: "power_saver", NightBatchOnly: true})
	if pol.AllowsHeavyWorkNow("plugged_in") {
		t.Fatalf("night-batch-only must not allow heavy work now")
	}
}
