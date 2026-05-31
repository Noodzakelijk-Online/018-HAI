export interface IWorkflowItem {
  id: string;
  title: string;
  description?: string;
  projectKey?: string;
  currentState: string;
  taskType: string;
  riskLevel: string;
  priorityScore: number;
  confidence: number;
  autonomyLevel: string;
  requiresApproval: boolean;
  approvalReason?: string;
  blockedReason?: string;
  nextAction?: string;
  sourceType?: string;
  sourceUri?: string;
  sourceLabel?: string;
  dueAt?: string;
  archived: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface IWorkflowChecklistItem {
  id: string;
  workflowId: string;
  label: string;
  status: string;
  position: number;
  requiresApproval: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface IWorkflowEvent {
  id: string;
  workflowId: string;
  eventType: string;
  fromState?: string;
  toState?: string;
  message?: string;
  trigger?: string;
  ruleApplied?: string;
  sourceUri?: string;
  actor?: string;
  createdAt: string;
}

export interface IWorkflowRecord {
  item: IWorkflowItem;
  checklist: IWorkflowChecklistItem[];
  events: IWorkflowEvent[];
}

export interface IWorkflowIntakeRequest {
  input: string;
  projectKey?: string;
  sourceType?: string;
  sourceUri?: string;
  sourceLabel?: string;
  trigger?: string;
  actor?: string;
}

export interface IWorkflowTransitionRequest {
  targetState: string;
  message?: string;
  approved?: boolean;
  actor?: string;
}

export interface IWorkflowChecklistUpdateRequest {
  status: string;
}

export interface IWorkflowCapability {
  id: string;
  name: string;
  status: string;
  implemented: string[];
  next: string[];
}

export interface IWorkflowOverview {
  capabilities: IWorkflowCapability[];
  states: string[];
  safetyRules: string[];
}
