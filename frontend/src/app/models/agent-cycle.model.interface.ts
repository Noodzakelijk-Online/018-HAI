import { IAmbientScan } from './ambient.model.interface';
import { IContextMemory } from './context-memory.model.interface';
import { IPursuitBrief, IPursuitDashboardDecision } from './pursuit.model.interface';
import {
  IWorkflowClaimRecoverySummary,
  IWorkflowDashboard,
  IWorkflowOpenLoopRunSummary,
  IWorkflowRunSummary,
} from './workflow.model.interface';

export interface IAgentCycleRunRequest {
  trigger?: string;
  limit?: number;
  skipSourceSync?: boolean;
  skipAmbient?: boolean;
}

export interface IAgentCyclePhaseError {
  phase: string;
  message: string;
}

export interface IAgentCycleWorkerStep {
  name: string;
  status: string;
  summary: string;
}

export interface IAgentCyclePursuitOperatingState {
  operatingMode: string;
  primaryLane: string;
  primaryAction: string;
  needsRobert: number;
  readyToMove: number;
  stuck: number;
  reviewDue: number;
  planningNeeded: number;
  completionCandidates: number;
  recentlyChanged: number;
  attentionTotal: number;
}

export interface IAgentCycleAppliedContext {
  memory: IContextMemory;
  score: number;
  explanation: string;
}

export interface IAgentCycleRunResult {
  trigger: string;
  status: string;
  startedAt: string;
  completedAt: string;
  steps: IAgentCycleWorkerStep[];
  errors: IAgentCyclePhaseError[];
  appliedContext?: IAgentCycleAppliedContext[];
  contextNote?: string;
  sourceSync?: {
    checked: number;
    due: number;
    completed: number;
    failed: number;
    skipped: number;
  };
  recovery?: IWorkflowClaimRecoverySummary;
  openLoops?: IWorkflowOpenLoopRunSummary;
  workflows?: IWorkflowRunSummary;
  ambientScan?: IAmbientScan;
  dashboard?: IWorkflowDashboard;
  pursuitBrief?: IPursuitBrief;
  pursuitDecisions?: IPursuitDashboardDecision[];
  pursuitOperatingState?: IAgentCyclePursuitOperatingState;
  nextAction: string;
  safetySummary: string;
  learningIds?: string[];
  learningNote?: string;
}
