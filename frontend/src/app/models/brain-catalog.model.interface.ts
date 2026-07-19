export type BrainCatalogStatus =
  | 'integrated_profile'
  | 'candidate'
  | 'compatibility_only'
  | 'reference_only'
  | 'license_review'
  | 'excluded'

export interface IBrainCatalogControlMapping {
  sourcePattern: string
  haiControl: string
  boundary: string
}

export interface IBrainCatalogEntry {
  id: string
  name: string
  upstreamUrl: string
  sourceCatalogUrl: string
  sourceCollection?: string
  status: BrainCatalogStatus
  category: string
  integrationMode: string
  capabilities: string[]
  recommendedFor: string[]
  requiresApproval: boolean
  localFirstCompatible: boolean
  activation: string
  rationale: string
  verifiedAt: string
  verificationNote: string
  controlMappings?: IBrainCatalogControlMapping[]
}

export interface IBrainCatalogSource {
  name: string
  url: string
  scope: string
}

export interface IBrainCatalogResponse {
  sourceCatalog: string
  discoverySources: IBrainCatalogSource[]
  verifiedAt: string
  entries: IBrainCatalogEntry[]
  activationPolicy: string
}
