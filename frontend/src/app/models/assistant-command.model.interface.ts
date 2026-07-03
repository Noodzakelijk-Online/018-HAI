import { IAgentCycleRunResult } from './agent-cycle.model.interface';
import { ICompletionPlan } from './task-plan.model.interface';

export interface IAssistantCommandRequest {
  message: string;
  projectKey?: string;
  automationId?: string;
  successCriteria?: string[];
  executeAllowed?: boolean;
  runCycle?: boolean;
  skipSourceSync?: boolean;
  skipAmbient?: boolean;
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
}
