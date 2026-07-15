export interface IProviderSummary {
  id: string
  name: string
  status: string
  claimLevel: string
  local: boolean
  models: number
}

export interface ILaneWinner {
  lane: string
  providerId: string
  modelId: string
  tokensPerSecond: number
  runs: number
}

export interface IModelIntelligenceOverview {
  providers: IProviderSummary[]
  lanes: string[]
  totalProfiles: number
  activeModels: number
  telemetryRuns: number
  cacheHits: number
  cacheMisses: number
  laneWinners: ILaneWinner[]
}

export interface IModelProfile {
  providerId: string
  modelId: string
  displayName: string
  architectureFamily: string
  lanes: string[]
  contextWindow: number
  local: boolean
  paid: boolean
  status: string
  claimLevel: string
  observedTokensPerSecond: number
  observedRuns: number
  observedFailures: number
  lastBenchmarkedAt?: string
}

export interface IBenchmarkResult {
  providerId: string
  modelId: string
  ok: boolean
  outputTokens: number
  durationMs: number
  tokensPerSecond: number
  claimLevel: string
  detail?: string
}

export interface IModelTelemetry {
  id: string
  providerId: string
  modelId: string
  lane: string
  operationId?: string
  inputTokens: number
  outputTokens: number
  durationMs: number
  tokensPerSecond: number
  ok: boolean
  cacheHit: boolean
  createdAt: string
}

export interface IOperationBudget {
  maximumInputTokens: number
  maximumOutputTokens: number
  maximumReasoningEffort: string
  maximumContextItems: number
  maximumSourceBytes: number
  contextStrategy: string
  cacheStrategy: string
  batchEligible: boolean
}

export interface IHardwareProfile {
  operatingSystem: string
  windowsVersion: string
  cpuCores: number
  gpuVendor: string
  npuVendor: string
  executionProviders: string[]
  powerMode: string
  batteryStatus: string
}

export interface IHardwareResponse {
  profile: IHardwareProfile
  selectedServingStack: string
}

export interface IPowerPolicy {
  mode: string
  allowHeavyWorkNow: boolean
  deferHeavyWorkOnBattery: boolean
  nightBatchOnly: boolean
}
