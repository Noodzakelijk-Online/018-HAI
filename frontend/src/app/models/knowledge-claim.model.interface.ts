export type KnowledgeVerificationStatus =
  | 'unverified'
  | 'source_supported'
  | 'schema_validated'
  | 'test_passed'
  | 'human_approved'
  | 'verified'
  | 'uncertain'
  | 'conflicting'
  | 'unsupported'
  | 'needs_review'

export type ClaimAssessmentStatus =
  | 'supported'
  | 'corroborated'
  | 'conflicting'
  | 'superseded'
  | 'needs_review'

export interface IClaimProvenance {
  referenceId?: string
  uri?: string
  sourceNodeId?: string
  contentDigest: string
  authority?: string
  capturedAt: string
  localOnly: boolean
}

export interface IKnowledgeClaim {
  id: string
  ownerIdentity: string
  workspaceId: string
  subject: string
  predicate: string
  object: string
  effectiveFrom: string
  effectiveUntil?: string
  observedAt: string
  verificationStatus: KnowledgeVerificationStatus
  provenance: IClaimProvenance[]
  provenanceDigest: string
  supersedesClaimIds?: string[]
  conflictsWithIds?: string[]
  sensitivity: 'public' | 'internal' | 'sensitive' | 'restricted'
  localOnly: boolean
  claimDigest: string
  createdAt: string
}

export interface IClaimAssessment {
  claimId: string
  subject: string
  predicate: string
  object: string
  status: ClaimAssessmentStatus
  effectiveAt: string
  observedBy: string
  reasons: string[]
  evidenceIds: string[]
  supportingClaimIds: string[]
  conflictingClaimIds: string[]
  supersedingClaimIds: string[]
  truncated: boolean
}

export interface IClaimReviewItem {
  claim: IKnowledgeClaim
  assessment: IClaimAssessment
}

export interface IClaimReviewQueue {
  items: IClaimReviewItem[]
  counts: Partial<Record<ClaimAssessmentStatus, number>>
  effectiveAt: string
  observedBy: string
  truncated: boolean
}

export interface IClaimLifecycle {
  claim: IKnowledgeClaim
  supersedes: IKnowledgeClaim[]
  supersededBy: IKnowledgeClaim[]
  conflicts: IKnowledgeClaim[]
  truncated: boolean
}

export interface ICorrectClaimRequest {
  workspaceId: string
  requestId: string
  correctedObject: string
  reason: string
  effectiveFrom?: string
}
