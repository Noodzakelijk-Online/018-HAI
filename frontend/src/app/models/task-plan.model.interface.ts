import { ILLMGenerationResult, ILLMRouteDecision } from './llm-policy.model.interface';
import { IRankedMemory } from './context-memory.model.interface';
import { IRankedExtraction } from './connected-source.model.interface';
import { IVerificationClaim } from './verification.model.interface';

export interface ITaskPlanRequest {
  request: string;
  projectKey?: string;
  successCriteria?: string[];
  executeAllowed?: boolean;
  humanApproved?: boolean;
  approvalNote?: string;
}

export interface IIntakeAnalysis {
  taskType: string;
  riskLevel: string;
  difficulty: number;
  requiredReasoning: string;
  successCriteria: string[];
  needsMemory: boolean;
  needsTools: boolean;
  needsDocuments: boolean;
  needsWebAccess: boolean;
  needsLocalExecution: boolean;
  needsApproval: boolean;
  reason: string;
}

export interface IContextPlan {
  strategy: string[];
  usedContext: IRankedMemory[];
  sourceContext: IRankedExtraction[];
  explanation: string;
}

export interface IValidationPlan {
  steps: string[];
  failurePolicy: string;
  completionGate: string;
}

export interface IExecutionPlan {
  planningSeparatedFromExecution: boolean;
  controlledExecutionMode: string;
  approvalRequiredFor: string[];
  auditEvents: string[];
}

export interface IExecutedAction {
  name: string;
  status: string;
  input?: string;
  output?: string;
  startedAt: string;
  endedAt: string;
}

export interface IExecutionResult {
  startedAt: string;
  completedAt: string;
  mode: string;
  output: string;
  verificationStatus: string;
  claims: IVerificationClaim[];
  evidenceCount: number;
  unsupportedClaims: number;
  llmGeneration?: ILLMGenerationResult;
  actions: IExecutedAction[];
  blockedReason?: string;
}

export interface IToolRouteDecision {
  selectedTools: string[];
  skippedTools: string[];
  blockedTools: string[];
  reason: string;
}

export interface ITaskStep {
  id: string;
  name: string;
  purpose: string;
  allowed: boolean;
  requiresApproval: boolean;
  status: string;
}

export interface IRiskAssessment {
  level: string;
  approvalRequired: boolean;
  approvalGranted: boolean;
  reasons: string[];
  allowedNow: boolean;
}

export interface IValidationResult {
  passed: boolean;
  status: string;
  checked: string[];
  failures: string[];
  nextAction: string;
  attemptNumber: number;
}

export interface IRetryPolicy {
  maxAttempts: number;
  escalationPath: string[];
  escalateWhen: string[];
  currentAttempt: number;
  retryAvailable: boolean;
}

export interface IReviewQueueItem {
  id: string;
  taskId: string;
  request: ITaskPlanRequest;
  reason: string;
  priority: string;
  status: string;
  decision?: string;
  resolutionNote?: string;
  createdAt: string;
  resolvedAt?: string;
}

export interface IApprovalDecision {
  approved: boolean;
  note?: string;
}

export interface IReviewResolutionResult {
  item: IReviewQueueItem;
  plan?: ICompletionPlan;
}

export interface ITaskEvent {
  at: string;
  stage: string;
  message: string;
}

export interface IMemoryUpdateProposal {
  kind: string;
  content: string;
  tags: string[];
  reason: string;
  confidence: number;
}

export interface ICompletionPlan {
  id: string;
  createdAt: string;
  request: string;
  projectKey?: string;
  realGoal: string;
  intake: IIntakeAnalysis;
  contextPlan: IContextPlan;
  modelDecision: ILLMRouteDecision;
  toolDecision: IToolRouteDecision;
  steps: ITaskStep[];
  riskAssessment: IRiskAssessment;
  validationPlan: IValidationPlan;
  validationResult: IValidationResult;
  executionPlan: IExecutionPlan;
  executionResult?: IExecutionResult;
  retryPolicy: IRetryPolicy;
  reviewQueueItem?: IReviewQueueItem;
  memoryUpdateProposals: IMemoryUpdateProposal[];
  lessonsLearned: IMemoryUpdateProposal[];
  storedMemoryIds: string[];
  events: ITaskEvent[];
  completionStatus: string;
}
