export interface IAgentRuntimeInfo {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  configured: boolean;
  executionEnabled: boolean;
  requiresApproval: boolean;
  readOnlyDefault: boolean;
  capabilities: string[];
  missingConfiguration?: string[];
  endpoint?: string;
}

export interface IAgentRuntimeHealth {
  runtimeId: string;
  status: string;
  reason: string;
  checkedAt: string;
  latencyMs: number;
}
