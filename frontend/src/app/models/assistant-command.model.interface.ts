import { IAgentCycleRunResult } from './agent-cycle.model.interface';
import { IPursuitMatchCandidate } from './pursuit.model.interface';
import { ICompletionPlan } from './task-plan.model.interface';

export interface IAssistantCommandRequest {
  message: string;
  projectKey?: string;
  pursuitId?: string;
  automationId?: string;
  mandateId?: string;
  successCriteria?: string[];
  includeRagflowCandidates?: boolean;
  executeAllowed?: boolean;
  runCycle?: boolean;
  skipSourceSync?: boolean;
  skipAmbient?: boolean;
}

export interface IAssistantCommandPursuitContext {
  pursuitId?: string;
  title?: string;
  mode: string;
  matched: boolean;
  createdCandidate?: boolean;
  awaitingAcceptance?: boolean;
  executionQueued?: boolean;
  score?: number;
  reasons?: string[];
  message?: string;
  matches?: IPursuitMatchCandidate[];
}

export interface IAssistantCommandAction {
  name: string;
  status: string;
  summary: string;
}

export interface IAssistantCommandResult {
  id: string;
  createdAt: string;
  intent: string;
  summary: string;
  nextAction: string;
  safetySummary: string;
  actions: IAssistantCommandAction[];
  reviewRequired: boolean;
  plan?: ICompletionPlan;
  agentCycle?: IAgentCycleRunResult;
  pursuit?: IAssistantCommandPursuitContext;
}
