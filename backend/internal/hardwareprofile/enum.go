// Package hardwareprofile provides truthful local hardware detection and
// serving-stack selection (§18). Detection never claims Windows ML on
// non-Windows, never claims NPU/GPU support without detection or explicit
// operator config, and returns unknown/unavailable in CI/dev where appropriate.
package hardwareprofile

import "fmt"

func parseEnum[T ~string](kind, v string, valid []T) (T, error) {
	for _, x := range valid {
		if string(x) == v {
			return x, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("hardwareprofile: invalid %s %q", kind, v)
}

// ExecutionProvider is an ONNX/local execution provider label (§18).
type ExecutionProvider string

const (
	EPWindowsML          ExecutionProvider = "windows_ml"
	EPOnnxCPU            ExecutionProvider = "onnx_runtime_cpu"
	EPOnnxCUDA           ExecutionProvider = "onnx_runtime_cuda"
	EPOnnxDirectML       ExecutionProvider = "onnx_runtime_directml"
	EPOnnxQNN            ExecutionProvider = "onnx_runtime_qnn"
	EPOnnxOpenVINO       ExecutionProvider = "onnx_runtime_openvino"
	EPOnnxVitisAI        ExecutionProvider = "onnx_runtime_vitisai"
	EPOnnxTensorRTRTX    ExecutionProvider = "onnx_runtime_nvtensorrtrtx"
	EPFoundryLocal       ExecutionProvider = "foundry_local"
	EPOllama             ExecutionProvider = "ollama"
	EPLMStudio           ExecutionProvider = "lm_studio"
	EPLlamaCPP           ExecutionProvider = "llama_cpp"
	EPLocalAI            ExecutionProvider = "localai"
	EPDSparkCompatible   ExecutionProvider = "dspark_compatible"
	EPCustomOpenAICompat ExecutionProvider = "custom_openai_compatible"
	EPWSL2CUDA           ExecutionProvider = "wsl2_cuda"
	EPUnknown            ExecutionProvider = "unknown"
)

func allExecutionProviders() []ExecutionProvider {
	return []ExecutionProvider{
		EPWindowsML, EPOnnxCPU, EPOnnxCUDA, EPOnnxDirectML, EPOnnxQNN, EPOnnxOpenVINO,
		EPOnnxVitisAI, EPOnnxTensorRTRTX, EPFoundryLocal, EPOllama, EPLMStudio, EPLlamaCPP, EPLocalAI,
		EPDSparkCompatible, EPCustomOpenAICompat, EPWSL2CUDA, EPUnknown,
	}
}

func (e ExecutionProvider) String() string { return string(e) }
func (e ExecutionProvider) IsValid() bool {
	_, err := parseEnum("executionProvider", string(e), allExecutionProviders())
	return err == nil
}
func ParseExecutionProvider(v string) (ExecutionProvider, error) {
	return parseEnum("executionProvider", v, allExecutionProviders())
}

// ServingStack is a model serving stack label (§18).
type ServingStack string

const (
	StackWindowsMLOnnx      ServingStack = "windows_ml_onnx"
	StackOnnxCPU            ServingStack = "onnx_runtime_cpu"
	StackOnnxCUDA           ServingStack = "onnx_runtime_cuda"
	StackOnnxQNN            ServingStack = "onnx_runtime_qnn"
	StackOnnxOpenVINO       ServingStack = "onnx_runtime_openvino"
	StackOnnxVitisAI        ServingStack = "onnx_runtime_vitisai"
	StackOnnxTensorRTRTX    ServingStack = "onnx_runtime_tensorrt_rtx"
	StackDirectMLLegacy     ServingStack = "directml_legacy_fallback"
	StackFoundryLocal       ServingStack = "foundry_local"
	StackWSL2CUDA           ServingStack = "wsl2_cuda"
	StackLlamaCPPGGUF       ServingStack = "llama_cpp_gguf"
	StackOllama             ServingStack = "ollama"
	StackLMStudio           ServingStack = "lm_studio"
	StackLocalAI            ServingStack = "localai"
	StackDSparkCompatible   ServingStack = "dspark_compatible"
	StackCustomOpenAICompat ServingStack = "custom_openai_compatible"
	StackCloudAPI           ServingStack = "cloud_api"
	StackUnknown            ServingStack = "unknown"
)

func allServingStacks() []ServingStack {
	return []ServingStack{
		StackWindowsMLOnnx, StackOnnxCPU, StackOnnxCUDA, StackOnnxQNN, StackOnnxOpenVINO,
		StackOnnxVitisAI, StackOnnxTensorRTRTX, StackDirectMLLegacy, StackFoundryLocal,
		StackWSL2CUDA, StackLlamaCPPGGUF, StackOllama, StackLMStudio, StackLocalAI, StackDSparkCompatible,
		StackCustomOpenAICompat, StackCloudAPI, StackUnknown,
	}
}

func (s ServingStack) String() string { return string(s) }
func (s ServingStack) IsValid() bool {
	_, err := parseEnum("servingStack", string(s), allServingStacks())
	return err == nil
}
func ParseServingStack(v string) (ServingStack, error) {
	return parseEnum("servingStack", v, allServingStacks())
}
