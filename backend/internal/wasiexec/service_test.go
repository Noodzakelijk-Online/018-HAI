package wasiexec

import "testing"

const testModuleHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidateRunnerAllowsOnlyLocalHosts(t *testing.T) {
	modules := []Module{{ID: "health", Name: "Health probe", File: "health.wasm", SHA256: testModuleHash}}

	for _, runner := range []string{
		"http://wasi-runner:8090",
		"http://localhost:8090",
		"http://host.docker.internal:8090",
		"http://127.0.0.1:8090",
		"http://[::1]:8090",
	} {
		s := NewService(nil, true, runner, "1234567890abcdef", modules)
		if s.configErr != "" {
			t.Fatalf("expected local runner %q to be accepted, got %q", runner, s.configErr)
		}
	}

	for _, runner := range []string{
		"http://8.8.8.8:8090",
		"http://192.168.1.50:8090",
		"http://runner.example.test:8090",
	} {
		s := NewService(nil, true, runner, "1234567890abcdef", modules)
		if s.configErr == "" {
			t.Fatalf("expected non-local runner %q to be rejected", runner)
		}
	}
}
