export interface IContextMemory {
  id?: string;
  projectKey?: string;
  kind: string;
  content: string;
  summary?: string;
  tags?: string;
  confidence: number;
  sourceUri?: string;
  sourceLabel?: string;
  contentHash?: string;
  archived: boolean;
  lastUsedAt?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface IContextMemoryRequest {
  projectKey?: string;
  kind: string;
  content: string;
  summary?: string;
  tags?: string[];
  confidence?: number;
  sourceUri?: string;
  sourceLabel?: string;
  archived?: boolean;
}

export interface IMemoryRetrieveRequest {
  query: string;
  projectKey?: string;
  limit?: number;
}

export interface IRankedMemory {
  memory: IContextMemory;
  score: number;
  explanation: string;
}

export interface IMemoryRetrieveResult {
  query: string;
  projectKey?: string;
  usedContext: IRankedMemory[];
  explanation: string;
}

export interface IMemoryExport {
  format: string;
  memories: IContextMemory[];
}
