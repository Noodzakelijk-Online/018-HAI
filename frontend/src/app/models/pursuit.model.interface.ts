import { ISourceExtraction } from './connected-source.model.interface';
import { IContextMemory } from './context-memory.model.interface';
import { IAutomationLaunchEvent } from './automation.model.interface';
import {
  IVerificationClaim,
  IVerificationEvidence,
  IVerificationRun,
} from './verification.model.interface';
import {
  IWorkflowDecision,
  IWorkflowEvidenceClaim,
  IWorkflowEvent,
  IWorkflowItem,
  IWorkflowOpenLoop,
  IWorkflowProposal,
  IWorkflowQualityGate,
  IWorkflowSourceLink,
  IWorkflowTransition,
} from './workflow.model.interface';

export interface IPursuit {
  id: string;
  ownerIdentity?: string;
  title: string;
  description?: string;
  whyItMatters?: string;
  projectKey?: string;
  domain?: string;
  desiredOutcome?: string;
  currentStateSummary?: string;
  status: string;
  priorityScore: number;
  riskLevel: string;
  confidence: number;
  autonomyLevel: string;
  needCategory?: string;
  sourceOfCreation?: string;
  nextRecommendedAction?: string;
  completionDefinition?: string;
  completionState: string;
  lastActivityAt?: string;
  nextReviewAt?: string;
  archived: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface IPursuitLink {
  id: string;
  pursuitId: string;
  linkType: string;
  linkId: string;
  relationship: string;
  sourceUri?: string;
  sourceLabel?: string;
  confidence: number;
  createdAt: string;
}

export interface IPursuitActivity {
  id: string;
  pursuitId: string;
  eventType: string;
  message?: string;
  actor?: string;
  sourceType?: string;
  sourceId?: string;
  sourceUri?: string;
  createdAt: string;
}

export interface IPursuitReviewRequest {
  action?: 'complete' | 'reviewed' | 'done' | 'snooze' | 'postpone';
  actor?: string;
  note?: string;
  nextReviewAt?: string;
  snoozeDays?: number;
}

export interface IPursuitPlanRequest {
  input?: string;
  actor?: string;
  requiresReview?: boolean;
  reviewReason?: string;
}

export interface IPursuitListItem {
  pursuit: IPursuit;
  needsRobert: number;
  blocked: number;
  openLoops: number;
  decisionCards: number;
  linkedEvidence: number;
  timelineItems: number;
  completionCandidate: boolean;
  currentState?: string;
  whatChanged?: string;
  nextAction?: string;
  stale: boolean;
  reviewDue: boolean;
  planningNeeded: boolean;
}

export interface IPursuitDashboard {
  counts: Record<string, number>;
  decisionQueue: IPursuitDashboardDecision[];
  needsRobert: IPursuitListItem[];
  vaReady: IPursuitListItem[];
  systemReady: IPursuitListItem[];
  blocked: IPursuitListItem[];
  stale: IPursuitListItem[];
  reviewDue: IPursuitListItem[];
  planningNeeded: IPursuitListItem[];
  recentlyChanged: IPursuitListItem[];
  highRisk: IPursuitListItem[];
  completionCandidates: IPursuitListItem[];
}

export interface IPursuitDashboardDecision {
  pursuit: IPursuit;
  decision: IPursuitDecision;
  currentState?: string;
  nextAction?: string;
  blocked: number;
  evidenceLine?: string;
}

export interface IPursuitBriefCard {
  queue: string;
  pursuitId: string;
  title: string;
  action: string;
  context: string;
  riskLevel: string;
  evidenceLine: string;
  needsRobert: boolean;
}

export interface IPursuitBrief {
  generatedAt: string;
  operatingMode: string;
  summary: string;
  primaryAction: string;
  needsRobert: number;
  readyToMove: number;
  stuck: number;
  reviewDue: number;
  planningNeeded: number;
  completionCandidates: number;
  recentlyChanged: number;
  cards: IPursuitBriefCard[];
}

export interface IPursuitAction {
  label: string;
  owner: string;
  riskLevel: string;
  requiresApproval: boolean;
  reason: string;
  workflowId?: string;
  yesLabel?: string;
  noLabel?: string;
}

export interface IPursuitBlocker {
  label: string;
  reason: string;
  owner: string;
  workflowId?: string;
  followUpAt?: string;
}

export interface IPursuitSummary {
  currentState: string;
  whatChanged: string;
  needsRobert: number;
  blocked: number;
  openLoops: number;
  robertActions: number;
  vaReadyActions: number;
  systemReadyActions: number;
  waitingActions: number;
  decisionCards: number;
  timelineItems: number;
  taskRuns: number;
  linkedEvidence: number;
  verificationRuns: number;
  runtimeAttempts: number;
  qualityGatesNeedingReview: number;
  confidence: number;
  planningNeeded: boolean;
  reviewDue: boolean;
  completionCandidate: boolean;
}

export interface IPursuitOperationalDigest {
  primaryLane: string;
  headline: string;
  recommendedAction: string;
  robertLine: string;
  delegationLine: string;
  systemLine: string;
  waitingLine: string;
  blockerLine: string;
  evidenceLine: string;
  runtimeLine: string;
  sourceLine: string;
  verificationLine: string;
  riskLine: string;
  needsRobert: number;
  vaReady: number;
  systemReady: number;
  waiting: number;
  blocked: number;
  evidence: number;
  runtimeAttempts: number;
  verificationRuns: number;
  openLoops: number;
}

export interface IPursuitActionQueues {
  needsRobert: IPursuitAction[];
  vaReady: IPursuitAction[];
  systemReady: IPursuitAction[];
  waiting: IPursuitAction[];
}

export interface IPursuitSourceItem {
  id: string;
  sourceId: string;
  externalId: string;
  projectKey?: string;
  itemType?: string;
  title?: string;
  sourceUri?: string;
  metadata?: string;
  fetchedAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface IPursuitAutomation {
  id: string;
  name: string;
  runtimeType?: string;
  launchType?: string;
  status?: string;
  lastLaunchAt?: string;
  lastFailureReason?: string;
}

export interface IPursuitTaskRun {
  workflowId: string;
  workflowTitle: string;
  taskPlanId?: string;
  status: string;
  verificationStatus?: string;
  retryCount: number;
  maxRetries: number;
  lastRunAt?: string;
  nextRunAt?: string;
  lastWorkerError?: string;
  automationId?: string;
  needsReview: boolean;
}

export interface IPursuitDecision {
  id: string;
  workflowId?: string;
  workflowTitle?: string;
  decisionType: string;
  status: string;
  recommended: string;
  reason: string;
  riskLevel: string;
  evidenceUri?: string;
  evidenceLabel?: string;
  yesLabel: string;
  noLabel: string;
  yesConsequence: string;
  noConsequence: string;
  requiresApproval: boolean;
  approved: boolean;
  actor?: string;
  createdAt?: string;
}

export interface IPursuitTimelineItem {
  id: string;
  kind: string;
  title: string;
  message?: string;
  workflowId?: string;
  workflowTitle?: string;
  actor?: string;
  status?: string;
  riskLevel?: string;
  sourceUri?: string;
  sourceLabel?: string;
  needsReview: boolean;
  createdAt: string;
}

export interface IPursuitDetail {
  pursuit: IPursuit;
  links: IPursuitLink[];
  activity: IPursuitActivity[];
  workflows: IWorkflowItem[];
  openLoops: IWorkflowOpenLoop[];
  proposals: IWorkflowProposal[];
  qualityGates: IWorkflowQualityGate[];
  decisions: IWorkflowDecision[];
  decisionQueue: IPursuitDecision[];
  transitions: IWorkflowTransition[];
  sourceLinks: IWorkflowSourceLink[];
  events: IWorkflowEvent[];
  timeline: IPursuitTimelineItem[];
  evidence: IWorkflowEvidenceClaim[];
  memories: IContextMemory[];
  taskRuns: IPursuitTaskRun[];
  verificationRuns: IVerificationRun[];
  verificationClaims: IVerificationClaim[];
  verificationEvidence: IVerificationEvidence[];
  automations: IPursuitAutomation[];
  runtimeAttempts: IAutomationLaunchEvent[];
  sourceItems: IPursuitSourceItem[];
  sourceExtractions: ISourceExtraction[];
  nextActions: IPursuitAction[];
  actionQueues: IPursuitActionQueues;
  blockers: IPursuitBlocker[];
  approvalItems: IWorkflowItem[];
  summary: IPursuitSummary;
  operationalDigest: IPursuitOperationalDigest;
}

export interface IPursuitDelegationChecklistItem {
  label: string;
  status: string;
  required: boolean;
}

export interface IPursuitDelegationWorkItem {
  workflowId?: string;
  title: string;
  instructions: string;
  state?: string;
  dueAt?: string;
  checklist: IPursuitDelegationChecklistItem[];
}

export interface IPursuitDelegationSource {
  workflowId?: string;
  sourceType?: string;
  sourceUri: string;
  sourceLabel?: string;
  relationship?: string;
}

export interface IPursuitDelegationPackage {
  generatedAt: string;
  ready: boolean;
  status: string;
  reason: string;
  pursuitId: string;
  title: string;
  objective: string;
  whyItMatters?: string;
  currentState: string;
  completionDefinition?: string;
  riskLevel: string;
  workItems: IPursuitDelegationWorkItem[];
  sourceContext: IPursuitDelegationSource[];
  allowedActions: string[];
  blockedActions: string[];
  escalationRules: string[];
  deliveryRequirements: string[];
  outstandingRobertActions: IPursuitAction[];
}

export interface IPursuitEvidenceResolution {
  uri: string;
  kind: string;
  title: string;
  summary?: string;
  status?: string;
  sourceType?: string;
  sourceId?: string;
  sourceLabel?: string;
  workflowId?: string;
  needsReview: boolean;
  runtimeAttempt?: IAutomationLaunchEvent;
  pursuitLink?: IPursuitLink;
  timelineItem?: IPursuitTimelineItem;
  workflowEvidence?: IWorkflowEvidenceClaim;
  memory?: IContextMemory;
  sourceItem?: IPursuitSourceItem;
  sourceExtraction?: ISourceExtraction;
  verificationEvidence?: IVerificationEvidence;
  activity?: IPursuitActivity;
}

export interface IPursuitApprovalOverview {
  pursuit: IPursuit;
  decisionQueue: IPursuitDecision[];
  approvalItems: IWorkflowItem[];
  actions: IPursuitAction[];
  blockers: IPursuitBlocker[];
  summary: IPursuitSummary;
  counts: Record<string, number>;
}

export interface IPursuitCreateRequest {
  title: string;
  ownerIdentity?: string;
  description?: string;
  whyItMatters?: string;
  projectKey?: string;
  domain?: string;
  desiredOutcome?: string;
  currentStateSummary?: string;
  status?: string;
  priorityScore?: number;
  riskLevel?: string;
  confidence?: number;
  autonomyLevel?: string;
  needCategory?: string;
  sourceOfCreation?: string;
  nextRecommendedAction?: string;
  completionDefinition?: string;
  nextReviewAt?: string;
}

export interface IPursuitUpdateRequest extends Partial<IPursuitCreateRequest> {
  completionState?: string;
  archived?: boolean;
  actor?: string;
}

export interface IPursuitIntakeRequest {
  input: string;
  projectKey?: string;
  automationId?: string;
  sourceType?: string;
  sourceId?: string;
  sourceUri?: string;
  sourceLabel?: string;
  contentType?: string;
  sender?: string;
  receivedAt?: string;
  trigger?: string;
  actor?: string;
  requiresReview?: boolean;
  reviewReason?: string;
}

export interface IPursuitAutoLinkResult {
  linked: boolean;
  created?: boolean;
  pursuitId?: string;
  score: number;
  reasons?: string[];
  message?: string;
  links?: IPursuitLink[];
}

export interface IPursuitRoutedIntakeResult {
  mode: string;
  matched: boolean;
  createdCandidate: boolean;
  pursuitId?: string;
  score?: number;
  reasons?: string[];
  message?: string;
  matches?: IPursuitMatchCandidate[];
  detail?: IPursuitDetail;
  autoLink?: IPursuitAutoLinkResult;
}

export interface IPursuitDecisionResolutionRequest {
  decisionId: string;
  decisionType?: string;
  approved: boolean;
  reason?: string;
  note?: string;
  evidenceUri?: string;
  evidenceLabel?: string;
  actor?: string;
}

export interface IPursuitLinkRequest {
  linkType: string;
  linkId: string;
  relationship?: string;
  sourceUri?: string;
  sourceLabel?: string;
  confidence?: number;
  actor?: string;
}

export interface IPursuitMatchRequest {
  input?: string;
  projectKey?: string;
  sourceType?: string;
  sourceId?: string;
  sourceUri?: string;
  limit?: number;
}

export interface IPursuitMatchCandidate {
  pursuit: IPursuit;
  score: number;
  reasons: string[];
  confidence: string;
}
