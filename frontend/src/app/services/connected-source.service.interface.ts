import { Observable } from 'rxjs';
import {
  IConnectedSource,
  ICreateSourceRequest,
  IImportRequest,
  ISourceAuditLog,
  ISourceConnector,
  ISourceConnectionHealth,
  ISourceExtraction,
  ISourceSearchRequest,
  ISourceSearchResult,
  ISourceSyncJob,
  ISourceSyncResult,
  IKnowledgeGraphResult,
  IScheduledSyncRun,
} from '../models/connected-source.model.interface';

export interface IConnectedSourceService {
  connectors(): Observable<ISourceConnector[]>;
  sources(includeDisabled: boolean): Observable<IConnectedSource[]>;
  connectionHealth(sourceId: string): Observable<ISourceConnectionHealth>;
  syncJobs(sourceId?: string): Observable<ISourceSyncJob[]>;
  createSource(request: ICreateSourceRequest): Observable<IConnectedSource>;
  startGoogleOAuth(sourceId: string): Observable<{ authorizeUrl: string }>;
  sync(sourceId: string, request: IImportRequest): Observable<ISourceSyncResult>;
  transcribe(sourceId: string): Observable<ISourceSyncResult>;
  extractDocuments(sourceId: string): Observable<ISourceSyncResult>;
  runDueScheduledSyncs(): Observable<IScheduledSyncRun>;
  reindex(sourceId: string): Observable<ISourceSyncResult>;
  pause(sourceId: string): Observable<IConnectedSource>;
  resume(sourceId: string): Observable<IConnectedSource>;
  revoke(sourceId: string): Observable<IConnectedSource>;
  search(request: ISourceSearchRequest): Observable<ISourceSearchResult>;
  knowledgeGraph(projectKey: string, includeArchived: boolean, includeSensitive: boolean): Observable<IKnowledgeGraphResult>;
  extractions(projectKey: string, includeArchived: boolean): Observable<ISourceExtraction[]>;
  updateExtraction(id: string, extraction: Partial<ISourceExtraction>): Observable<ISourceExtraction>;
  archiveExtraction(id: string): Observable<ISourceExtraction>;
  deleteExtraction(id: string): Observable<void>;
  auditLogs(sourceId?: string, limit?: number): Observable<ISourceAuditLog[]>;
}
