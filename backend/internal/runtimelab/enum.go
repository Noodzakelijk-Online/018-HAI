// Package runtimelab is HAI's runtime control plane (§15). HAI does not rebuild
// Hermes/OpenClaw/Odysseus; it becomes the control plane above them. Every
// runtime adapter reports truthful health, exact setup requirements when
// unavailable, and NEVER claims execution unless the runtime actually ran. The
// only runtime that actually executes in this phase is the local safe worker;
// all others are honest contracts until real credentials/endpoints exist.
package runtimelab

import "fmt"

func parseEnum[T ~string](kind, v string, valid []T) (T, error) {
	for _, x := range valid {
		if string(x) == v {
			return x, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("runtimelab: invalid %s %q", kind, v)
}

// RuntimeKind classifies a runtime target (§15).
type RuntimeKind string

const (
	KindLocalSafeWorker RuntimeKind = "local_safe_worker"
	KindAgentRuntime    RuntimeKind = "agent_runtime" // Hermes/OpenClaw/Odysseus
	KindBrowser         RuntimeKind = "browser_contract"
	KindLocalScript     RuntimeKind = "local_script_contract"
	KindMCPTool         RuntimeKind = "mcp_tool_contract"
)

func allRuntimeKinds() []RuntimeKind {
	return []RuntimeKind{KindLocalSafeWorker, KindAgentRuntime, KindBrowser, KindLocalScript, KindMCPTool}
}

func (k RuntimeKind) String() string { return string(k) }
func (k RuntimeKind) IsValid() bool {
	_, err := parseEnum("runtimeKind", string(k), allRuntimeKinds())
	return err == nil
}
func ParseRuntimeKind(v string) (RuntimeKind, error) {
	return parseEnum("runtimeKind", v, allRuntimeKinds())
}

// RuntimeAttemptStatus is the status of a runtime execution/self-test attempt.
type RuntimeAttemptStatus string

const (
	AttemptPending       RuntimeAttemptStatus = "pending"
	AttemptRunning       RuntimeAttemptStatus = "running"
	AttemptSucceeded     RuntimeAttemptStatus = "succeeded"
	AttemptFailed        RuntimeAttemptStatus = "failed"
	AttemptBlocked       RuntimeAttemptStatus = "blocked"
	AttemptSetupRequired RuntimeAttemptStatus = "setup_required"
	AttemptInconclusive  RuntimeAttemptStatus = "inconclusive"
)

func allAttemptStatuses() []RuntimeAttemptStatus {
	return []RuntimeAttemptStatus{
		AttemptPending, AttemptRunning, AttemptSucceeded, AttemptFailed,
		AttemptBlocked, AttemptSetupRequired, AttemptInconclusive,
	}
}

func (s RuntimeAttemptStatus) String() string { return string(s) }
func (s RuntimeAttemptStatus) IsValid() bool {
	_, err := parseEnum("runtimeAttemptStatus", string(s), allAttemptStatuses())
	return err == nil
}
func ParseRuntimeAttemptStatus(v string) (RuntimeAttemptStatus, error) {
	return parseEnum("runtimeAttemptStatus", v, allAttemptStatuses())
}
