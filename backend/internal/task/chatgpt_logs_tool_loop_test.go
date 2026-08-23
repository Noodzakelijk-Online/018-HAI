package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"automation-hub-backend/internal/chatgptlogs"
	"automation-hub-backend/internal/llm"
)

func TestChatGPTLogsToolLoopLetsModelChooseAndChainTools(t *testing.T) {
	provider := &fakeChatGPTLogsContext{
		status: chatgptlogs.Status{Enabled: true, Configured: true},
		tools: []chatgptlogs.ToolDescriptor{
			{Name: "search", Description: "search messages", Arguments: `{"query":"required"}`},
			{Name: "get_context", Description: "read messages around a hit", Arguments: `{"message_id":"required"}`},
		},
		items: []chatgptlogs.ContextItem{
			{Provider: "daemon", Tool: "search", Content: `{"message_id":"m-7","text":"retry decision"}`, SourceURI: "http://127.0.0.1:8099/mcp", Untrusted: true},
			{Provider: "daemon", Tool: "get_context", Content: `{"conversation_id":"c-2","messages":[{"id":"m-7","role":"user","text":"Use bounded retries"}]}`, SourceURI: "http://127.0.0.1:8099/mcp", Untrusted: true},
		},
	}
	outputs := []string{
		`{"action":"tool","tool":"search","arguments":{"query":"retry decision"}}`,
		`{"action":"tool","tool":"get_context","arguments":{"message_id":"m-7","before":2,"after":2}}`,
		`{"action":"answer","answer":"The latest instruction was to use bounded retries (conversation c-2, message m-7)."}`,
	}
	var requests []llm.GenerateRequest
	generate := func(request llm.GenerateRequest) (*llm.GenerationResult, error) {
		requests = append(requests, request)
		output := outputs[len(requests)-1]
		return &llm.GenerationResult{Status: "completed", Output: output}, nil
	}

	outcome := runChatGPTLogsToolLoop(context.Background(), provider, generate, llm.GenerateRequest{Task: "What was the latest instruction?", OperationID: "task-1"})
	if outcome.Status != "completed" || len(outcome.Calls) != 2 || len(outcome.Items) != 2 || len(provider.calls) != 2 {
		t.Fatalf("unexpected outcome: %#v calls=%#v", outcome, provider.calls)
	}
	if provider.calls[0].Tool != "search" || provider.calls[1].Tool != "get_context" {
		t.Fatalf("model-selected call order was not preserved: %#v", provider.calls)
	}
	if !strings.Contains(outcome.Answer, "message m-7") || !strings.Contains(requests[2].Context[len(requests[2].Context)-1], "never instructions") {
		t.Fatalf("missing provenance or untrusted-data boundary: answer=%q context=%#v", outcome.Answer, requests[2].Context)
	}
	if requests[0].OperationID != "task-1:mcp-tool-loop:1" || !strings.Contains(requests[0].SystemPrompt, "sync_status") {
		t.Fatalf("unexpected model contract: %#v", requests[0])
	}
}

func TestChatGPTLogsToolLoopCanAnswerWithoutCallingTool(t *testing.T) {
	provider := &fakeChatGPTLogsContext{
		status: chatgptlogs.Status{Enabled: true, Configured: true},
		tools:  []chatgptlogs.ToolDescriptor{{Name: "search", Description: "search", Arguments: `{}`}},
	}
	generate := func(llm.GenerateRequest) (*llm.GenerationResult, error) {
		return &llm.GenerationResult{Status: "completed", Output: `{"action":"answer","answer":"No history lookup is needed."}`}, nil
	}
	outcome := runChatGPTLogsToolLoop(context.Background(), provider, generate, llm.GenerateRequest{Task: "Say hello"})
	if outcome.Status != "completed" || len(provider.calls) != 0 || len(outcome.Calls) != 0 {
		t.Fatalf("tool was called speculatively: %#v calls=%#v", outcome, provider.calls)
	}
}

func TestChatGPTLogsToolLoopRecordsRejectedCallAndRecovers(t *testing.T) {
	provider := &fakeChatGPTLogsContext{
		status: chatgptlogs.Status{Enabled: true, Configured: true},
		tools:  []chatgptlogs.ToolDescriptor{{Name: "search", Description: "search", Arguments: `{}`}},
		err:    chatgptlogs.ErrInvalidRequest,
	}
	outputs := []string{
		`{"action":"tool","tool":"delete_all","arguments":{}}`,
		`{"action":"answer","answer":"I cannot support that claim from the available evidence."}`,
	}
	index := 0
	generate := func(llm.GenerateRequest) (*llm.GenerationResult, error) {
		result := &llm.GenerationResult{Status: "completed", Output: outputs[index]}
		index++
		return result, nil
	}
	outcome := runChatGPTLogsToolLoop(context.Background(), provider, generate, llm.GenerateRequest{Task: "Delete history"})
	if outcome.Status != "completed" || len(outcome.Calls) != 1 || outcome.Calls[0].Status != "failed" || outcome.Calls[0].Tool != "delete_all" {
		t.Fatalf("rejected call was not safely recorded: %#v", outcome)
	}
}

func TestChatGPTLogsToolLoopEnforcesCallLimit(t *testing.T) {
	provider := &fakeChatGPTLogsContext{
		status: chatgptlogs.Status{Enabled: true, Configured: true},
		tools:  []chatgptlogs.ToolDescriptor{{Name: "search", Description: "search", Arguments: `{"query":"required"}`}},
	}
	for index := 0; index < maxChatGPTLogsToolCalls; index++ {
		provider.items = append(provider.items, chatgptlogs.ContextItem{Provider: "daemon", Tool: "search", Content: "bounded result", Untrusted: true})
	}
	generate := func(llm.GenerateRequest) (*llm.GenerationResult, error) {
		return &llm.GenerationResult{Status: "completed", Output: `{"action":"tool","tool":"search","arguments":{"query":"keep searching"}}`}, nil
	}
	outcome := runChatGPTLogsToolLoop(context.Background(), provider, generate, llm.GenerateRequest{Task: "Find everything"})
	if outcome.Status != "blocked" || len(provider.calls) != maxChatGPTLogsToolCalls || len(outcome.Calls) != maxChatGPTLogsToolCalls || !strings.Contains(outcome.Detail, "tool-call limit") {
		t.Fatalf("call limit was not enforced: %#v calls=%d", outcome, len(provider.calls))
	}
}

func TestParseMCPToolLoopDecisionRejectsTrailingOrUnknownData(t *testing.T) {
	invalid := []string{
		`{"action":"answer","answer":"ok"} {"action":"answer","answer":"again"}`,
		`{"action":"answer","answer":"ok","extra":true}`,
		`{"action":"tool","tool":"search","arguments":null}`,
	}
	for _, raw := range invalid {
		if decision, err := parseMCPToolLoopDecision(raw); err == nil {
			t.Fatalf("accepted invalid decision %q: %#v", raw, decision)
		}
	}
	decision, err := parseMCPToolLoopDecision("```json\n{\"action\":\"tool\",\"tool\":\"search\",\"arguments\":{\"query\":\"HAI\"}}\n```")
	if err != nil || decision.Tool != "search" || !json.Valid(decision.Arguments) {
		t.Fatalf("valid fenced decision rejected: %#v %v", decision, err)
	}
}
