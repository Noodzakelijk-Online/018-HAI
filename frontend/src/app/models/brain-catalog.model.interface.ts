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

export interface IBrainCatalogUpstreamReview {
  id: string
  name: string
  upstreamUrl: string
  checkedAt: string
  available: boolean
  archived: boolean
  license?: string
  defaultBranch?: string
  pushedAt?: string
  message: string
  disposition: BrainCatalogStatus
  readiness: string
  readinessReason: string
  requiredGates?: string[]
}

export interface IBrainCatalogOSSInsightReview {
  checkedAt: string
  sourceUrl: string
  available: boolean
  expectedTotal: number
  currentTotal: number
  newCollections?: string[]
  missingExpected?: string[]
  message: string
}

export interface IBrainCatalogOSSInsightDiscovery {
  collection: string
  repository: string
  sourceUrl: string
  rationale: string
}

export interface IBrainCatalogOSSInsightDiscoveryReport {
  checkedAt: string
  sourceUrl: string
  available: boolean
  cached: boolean
  collectionsScreened: number
  candidateCollections: number
  collectionsChecked: number
  repositoriesChecked: number
  knownProfileHits: number
  discoveries?: IBrainCatalogOSSInsightDiscovery[]
  missingCollections?: string[]
  unavailableCollections?: string[]
  discoveriesTruncated: boolean
  message: string
}

export interface IBrainCatalogCapabilityRecommendation {
  id: string
  name: string
  status: BrainCatalogStatus
  role: string
  rationale: string
  requiresApproval: boolean
  activation: string
  score: number
  reasons: string[]
  nextStep: string
}

export interface IBrainCatalogCapabilityRecommendationResponse {
  need: string
  recommendations: IBrainCatalogCapabilityRecommendation[]
  message: string
}

export interface IBrainCatalogSource {
  name: string
  url: string
  scope: string
}

export type BrainCatalogCollectionDisposition =
  | 'represented_in_catalog'
  | 'review_candidate'
  | 'reference_only'
  | 'not_adopted'

export interface IBrainCatalogCollectionScreeningEntry {
  collection: string
  page: number
  disposition: BrainCatalogCollectionDisposition
  relatedEntryIds?: string[]
  rationale: string
  sourceUrl: string
}

export interface IBrainCatalogCollectionScreening {
  total: number
  represented: number
  candidates: number
  reference: number
  deferred: number
  entries: IBrainCatalogCollectionScreeningEntry[]
}

export interface IBrainCatalogResponse {
  sourceCatalog: string
  discoverySources: IBrainCatalogSource[]
  verifiedAt: string
  entries: IBrainCatalogEntry[]
  collectionScreening: IBrainCatalogCollectionScreening
  activationPolicy: string
}
