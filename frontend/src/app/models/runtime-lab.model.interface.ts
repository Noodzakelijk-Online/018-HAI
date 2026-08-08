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
  discoveryState: string
  readinessLevel: string
  protocol?: string
  runtimeVersion?: string
  protocolValid: boolean
  identityVerified: boolean
  authenticated: boolean
  capabilities?: string[]
  evidenceSha256?: string
  durationMs: number
  detail: string
  checkedAt: string
}

export type RuntimeFeatureDisposition =
  | 'integrated_directly'
  | 'adapted_for_hai'
  | 'hai_native_reimplementation'
  | 'already_present'
  | 'consolidated_existing'
  | 'constrained_unsafe'
  | 'excluded_irrelevant'
  | 'excluded_incompatible_license'
  | 'deferred'
  | 'blocked_external'

export interface IRuntimeFeature {
  id: string
  name: string
  purpose: string
  behavior: string
  coverageAreas: string[]
  dependencies: string[]
  license: string
  securityImplications: string
  haiEquivalent?: string
  integrationApproach: string
  disposition: RuntimeFeatureDisposition
  implementationStatus: string
  testStatus: string
  documentationStatus: string
  exclusionReason?: string
  backlogPriority?: string
  requirements?: string[]
  recommendedPath?: string
  sourceUrls: string[]
}

export interface IRuntimeParityInventory {
  runtimeId: string
  project: string
  repositoryUrl: string
  defaultBranch: string
  reviewedRevision: string
  reviewedRelease?: string
  reviewedAt: string
  license: string
  licensePolicy: string
  readinessCeiling: string
  canonicalAuthority: string
  features: IRuntimeFeature[]
}

export interface IRuntimeParityOverview {
  requiredCoverageAreas: string[]
  inventories: IRuntimeParityInventory[]
  dispositionCounts: Record<string, number>
  implementationCounts: Record<string, number>
  generatedAt: string
}

export interface IRuntimeCapabilityCard {
  id: string
  runtimeId: string
  name: string
  purpose: string
  inputSchema: Record<string, unknown>
  outputSchema: Record<string, unknown>
  authenticationState: string
  availability: string
  runtimeLocation: string
  requiredAuthority: string[]
  riskLevel: string
  expectedCostEurMax: number
  costPolicy: string
  contextCost: string
  timeoutSeconds: number
  retryBehaviour: string
  reversibility: string
  approvalRequirements: string[]
  verificationMethod: string
  evidenceReturned: string[]
  readinessLevel: string
  readinessReason: string
  canInvoke: boolean
  canExecuteExternalEffect: boolean
  latestDiscovery?: IRuntimeProbe
  sourceFeatureIds: string[]
}

export interface IRuntimeCapabilityOverview {
  cards: IRuntimeCapabilityCard[]
  counts: Record<string, number>
  authority: string
  safetyNote: string
}
