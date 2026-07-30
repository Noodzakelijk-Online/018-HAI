import { Observable } from 'rxjs';
import {
  IConnectedSource,
  ICreateSourceRequest,
  IImportRequest,
  ISourceAuditLog,
  ISourceConnector,
  ISourceExtraction,
  ISourceSearchRequest,
  ISourceSearchResult,
  ISourceSyncJob,
  ISourceSyncResult,
  IScheduledSyncRun,
} from '../models/connected-source.model.interface';

export interface IConnectedSourceService {
  connectors(): Observable<ISourceConnector[]>;
  sources(includeDisabled: boolean): Observable<IConnectedSource[]>;
  syncJobs(sourceId?: string): Observable<ISourceSyncJob[]>;
  createSource(request: ICreateSourceRequest): Observable<IConnectedSource>;
  startGoogleOAuth(sourceId: string): Observable<{ authorizeUrl: string }>;
  sync(sourceId: string, request: IImportRequest): Observable<ISourceSyncResult>;
  transcribe(sourceId: string): Observable<ISourceSyncResult>;
  runDueScheduledSyncs(): Observable<IScheduledSyncRun>;
  reindex(sourceId: string): Observable<ISourceSyncResult>;
  pause(sourceId: string): Observable<IConnectedSource>;
  resume(sourceId: string): Observable<IConnectedSource>;
  revoke(sourceId: string): Observable<IConnectedSource>;
  search(request: ISourceSearchRequest): Observable<ISourceSearchResult>;
  extractions(projectKey: string, includeArchived: boolean): Observable<ISourceExtraction[]>;
  updateExtraction(id: string, extraction: Partial<ISourceExtraction>): Observable<ISourceExtraction>;
  archiveExtraction(id: string): Observable<ISourceExtraction>;
  deleteExtraction(id: string): Observable<void>;
  auditLogs(sourceId?: string): Observable<ISourceAuditLog[]>;
}
