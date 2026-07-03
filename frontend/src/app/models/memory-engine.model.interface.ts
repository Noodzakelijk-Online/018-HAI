import { IContextMemory, IRankedMemory } from './context-memory.model.interface';
import { IPursuitAutoLinkResult } from './pursuit.model.interface';
import { IWorkflowItem, IWorkflowOpenLoop } from './workflow.model.interface';

export interface IAIConversationArchive {
  id: string;
  platform: string;
  externalId: string;
  title: string;
  sourceUri: string;
  contentHash: string;
  revision: number;
  messageCount: number;
  preview?: string;
  capturedAt: string;
  lastMessageAt?: string;
  archived: boolean;
}

export interface IAIMemoryInsight {
  id: string;
  conversationId: string;
  revision: number;
  kind: string;
  text: string;
  projectKey?: string;
  owner?: string;
  robertNeeded: boolean;
  riskLevel: string;
  confidence: number;
  sourceUri: string;
  sourceLabel: string;
  needsReview: boolean;
  status: string;
  createdAt: string;
}

export interface IMemoryProjectSummary {
  projectKey: string;
  actions: number;
  decisions: number;
  risks: number;
  corrections?: number;
  open: number;
}

export interface ICommandDashboard {
  generatedAt: string;
  conversationCount: number;
  insightCount: number;
  needsRobert: IWorkflowItem[];
  delegateToVA: IAIMemoryInsight[];
  openLoops: IWorkflowOpenLoop[];
  contradictions: IAIMemoryInsight[];
  recentDecisions: IAIMemoryInsight[];
  sourceCorrections?: IContextMemory[];
  projects: IMemoryProjectSummary[];
  recentArchives: IAIConversationArchive[];
  warnings: string[];
}

export interface IMemoryEngineSearchResult {
  memory: {
    query: string;
    projectKey?: string;
    usedContext: IRankedMemory[];
    explanation: string;
  };
  facts: IAIMemoryInsight[];
}

export interface IAIConversationImportResult {
  conversation: IAIConversationArchive;
  insights: IAIMemoryInsight[];
  workflowIds: string[];
  pursuitLinks?: IPursuitAutoLinkResult[];
  deduplicated: boolean;
  warnings?: string[];
}
