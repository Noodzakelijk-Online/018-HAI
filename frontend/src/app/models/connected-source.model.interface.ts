export interface ISourceConnector {
  id: string;
  connectorKey: string;
  name: string;
  category: string;
  supportedModes: string;
  requiredScopes: string;
  localOnlyCapable: boolean;
  enabled: boolean;
}

export interface IConnectedSource {
  id: string;
  connectorKey: string;
  name: string;
  category: string;
  enabled: boolean;
  localOnly: boolean;
  syncFrequency: string;
  ingestionModes: string;
  permissions: string;
  excludePatterns: string;
  cursor?: string;
  status: string;
  lastSyncedAt?: string;
  revokedAt?: string;
}

export interface ICreateSourceRequest {
  connectorKey: string;
  name: string;
  category?: string;
  enabled: boolean;
  localOnly: boolean;
  syncFrequency?: string;
  ingestionModes?: string[];
  permissions?: string[];
  excludePatterns?: string[];
}

export interface IImportItem {
  externalId: string;
  title: string;
  content: string;
  sourceUri?: string;
  itemType?: string;
  projectKey?: string;
  metadata?: string;
}

export interface IImportRequest {
  mode?: string;
  items: IImportItem[];
}

export interface ISourceSyncJob {
  id: string;
  sourceId: string;
  mode: string;
  status: string;
  itemsSeen: number;
  itemsAdded: number;
  itemsUpdated: number;
  message?: string;
  startedAt: string;
  completedAt?: string;
}

export interface ISourceExtraction {
  id: string;
  sourceId: string;
  rawItemId: string;
  projectKey?: string;
  contentType: string;
  text: string;
  summary?: string;
  entities?: string;
  dates?: string;
  tasks?: string;
  decisions?: string;
  followUps?: string;
  sourceUri?: string;
  sourceLabel?: string;
  sensitive: boolean;
  uncertain: boolean;
  archived: boolean;
  updatedAt: string;
}

export interface ISourceSyncResult {
  job: ISourceSyncJob;
  extractions: ISourceExtraction[];
  message: string;
}

export interface ISourceSearchRequest {
  query: string;
  projectKey?: string;
  limit?: number;
  includeSensitive?: boolean;
}

export interface IRankedExtraction {
  extraction: ISourceExtraction;
  score: number;
  explanation: string;
}

export interface ISourceSearchResult {
  query: string;
  projectKey?: string;
  usedContext: IRankedExtraction[];
  explanation: string;
}

export interface ISourceAuditLog {
  id: string;
  sourceId?: string;
  action: string;
  message: string;
  createdAt: string;
}
