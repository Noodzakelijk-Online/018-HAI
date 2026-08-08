export type PlanGraphStatus = 'draft' | 'accepted'

export type PlanNodeStatus =
  | 'planned'
  | 'ready'
  | 'waiting'
  | 'blocked'
  | 'completed'
  | 'failed'
  | 'needs_approval'

export type PlanApprovalStatus = 'not_required' | 'pending' | 'approved' | 'rejected' | 'revoked'
export type PlanRisk = 'low' | 'medium' | 'high'
export type PlanOwnerType = 'robert' | 'hai' | 'agent' | 'external' | 'unassigned'

export interface IPlanResourceEstimate {
  resourceType: string
  label: string
  amount: number
  unit: string
  availableAmount?: number
  sourceLabel?: string
  sourceUri?: string
  confidence?: number
}

export interface IPlanConstraint {
  id: string
  type: 'deadline' | 'earliest_start' | 'capacity' | 'budget' | 'policy' | 'dependency' | 'other'
  label: string
  value?: string
  hard: boolean
  satisfied?: boolean
  sourceLabel?: string
  sourceUri?: string
  evidenceDigest?: string
}

export interface IPlanBinding {
  type: 'pursuit' | 'workflow' | 'task' | 'authorization' | 'source' | 'verification' | 'agent' | 'other'
  id: string
  label?: string
  uri?: string
  status?: string
}

export interface IPlanNode {
  id: string
  sequence: number
  title: string
  objective?: string
  status: PlanNodeStatus
  ownerType: PlanOwnerType
  ownerLabel?: string
  risk: PlanRisk
  approvalRequired: boolean
  approvalStatus: PlanApprovalStatus
  dependencyIds: string[]
  plannedStartAt?: string
  plannedEndAt?: string
  estimatedDurationMinutes?: number
  constraints: IPlanConstraint[]
  resourceEstimates: IPlanResourceEstimate[]
  bindings: IPlanBinding[]
  frameworkSelectionDigest?: string
  evidenceDigest?: string
  blockedReason?: string
  recoveryAction?: string
  transport: IPlanTransportNode
}

export interface IPlanEdge {
  id: string
  fromNodeId: string
  toNodeId: string
  type: 'finish_to_start' | 'start_to_start' | 'finish_to_finish' | 'conditional' | 'information' | 'other'
  required: boolean
  lagMinutes?: number
  condition?: string
}

export interface IPlanRepairRecord {
  id: string
  revision: number
  reason: string
  summary?: string
  actor?: string
  createdAt: string
  previousPlanDigest?: string
  resultingPlanDigest?: string
}

export interface IPlanRevisionSummary {
  id: string
  revision: number
  status: PlanGraphStatus
  planDigest: string
  reason?: string
  createdAt: string
  createdBy?: string
}

export interface IPlanApproval {
  required: boolean
  status: PlanApprovalStatus
  reason?: string
  policyRule?: string
  decidedAt?: string
  decidedBy?: string
}

export interface IPlanGraph {
  id: string
  ownerIdentity?: string
  title: string
  objective: string
  status: PlanGraphStatus
  risk: PlanRisk
  revision: number
  planDigest: string
  previousPlanId?: string
  successCriteria: string[]
  nodes: IPlanNode[]
  edges: IPlanEdge[]
  criticalPathNodeIds: string[]
  constraints: IPlanConstraint[]
  resourceEstimates: IPlanResourceEstimate[]
  bindings: IPlanBinding[]
  approval: IPlanApproval
  frameworkSelectionDigest?: string
  evidenceDigest?: string
  contextSnapshotDigest?: string
  authorizationReceiptId?: string
  verificationId?: string
  plannedStartAt?: string
  plannedEndAt?: string
  deadlineAt?: string
  createdAt: string
  updatedAt: string
  acceptedAt?: string
  completedAt?: string
  revisions: IPlanRevisionSummary[]
  repairHistory: IPlanRepairRecord[]
  canExecute: boolean
}

export interface IPlanGraphListResponse {
  plans: IPlanGraph[]
}

export interface IPlanPreviewRequest {
  planId?: string
  idempotencyKey: string
  title: string
  nodes: IPlanTransportNode[]
  edges: IPlanTransportEdge[]
}

export interface IPlanAcceptRequest {
  expectedRevision: number
  expectedDigest: string
}

export interface IPlanReplanRequest {
  expectedRevision: number
  expectedDigest: string
  idempotencyKey: string
  title: string
  nodes: IPlanTransportNode[]
  edges: IPlanTransportEdge[]
  reason: string
  trigger: string
}

export interface IPlanTransportBindings {
  pursuitId?: string
  workflowId?: string
  taskId?: string
  agentId?: string
}

export interface IPlanTransportNode {
  id: string
  type: string
  title: string
  owner: string
  status: 'planned' | 'ready' | 'blocked' | 'waiting' | 'needs_approval' | 'completed' | 'failed'
  estimatedMinutes: number
  estimatedCostEur: number
  earliestStart?: string
  deadline?: string
  risk: PlanRisk
  approvalState: 'not_required' | 'required' | 'granted' | 'rejected'
  frameworkDigest?: string
  evidenceDigest?: string
  bindings: IPlanTransportBindings
}

export interface IPlanTransportEdge {
  id: string
  from: string
  to: string
  type: string
  lagMinutes?: number
}
