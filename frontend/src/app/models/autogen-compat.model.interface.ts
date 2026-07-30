export interface IAutoGenCompatibilityEvent {
  id: string
  type: 'message' | 'handoff' | 'tool_call' | 'tool_result' | 'approval_request' | 'task_started' | 'task_completed' | 'task_failed' | 'termination'
  agent?: string
  correlationId?: string
  summary: string
  occurredAt?: string
}

export interface IAgentFrameworkMigrationStep {
  order: number
  haiControl: string
  agentFrameworkRole: string
  requiredEvents?: string[]
  gate: string
}

export interface IAgentFrameworkMigrationPlan {
  target: string
  preview: {
    workloadId: string
    openLoops: Array<{ kind: string; summary: string; recovery: string }>
    recommendedControls: Array<{ control: string; reason: string }>
    requiresReview: boolean
    executionAllowed: boolean
    persistenceAllowed: boolean
  }
  steps: IAgentFrameworkMigrationStep[]
  blockedUntil: string[]
  executionAllowed: boolean
  frameworkRuntimeDetected: boolean
  scope: string
}
