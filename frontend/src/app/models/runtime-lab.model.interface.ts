export interface IRuntimeInfo {
  id: string
  displayName: string
  kind: string
  description: string
}

export interface ISetupRequirement {
  step: string
  detail: string
}

export interface IRuntimeAttempt {
  id: string
  runtimeId: string
  operationId?: string
  status: string
  detail: string
  boundedOutput?: string
  verificationPassed: boolean
  createdAt: string
}

export interface IRuntimeSummary {
  info: IRuntimeInfo
  status: string
  claimLevel: string
  canExecute: boolean
  capabilities: string[]
  setupRequirements?: ISetupRequirement[]
  lastAttempt?: IRuntimeAttempt
}

export interface IRuntimeProbe {
  runtimeId: string
  status: string
  durationMs: number
  detail: string
  checkedAt: string
}
