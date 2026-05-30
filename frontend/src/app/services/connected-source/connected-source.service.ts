import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IConnectedSourceService } from '../connected-source.service.interface';
import {
  IConnectedSource,
  ICreateSourceRequest,
  IImportRequest,
  ISourceAuditLog,
  ISourceConnector,
  ISourceExtraction,
  ISourceSearchRequest,
  ISourceSearchResult,
  ISourceSyncResult,
} from '../../models/connected-source.model.interface';

@Injectable({
  providedIn: 'root',
})
export class ConnectedSourceService implements IConnectedSourceService {
  private apiUrl = '/api/v1/sources';

  constructor(private http: HttpClient) {}

  connectors(): Observable<ISourceConnector[]> {
    return this.http.get<ISourceConnector[]>(`${this.apiUrl}/connectors`);
  }

  sources(includeDisabled: boolean): Observable<IConnectedSource[]> {
    return this.http.get<IConnectedSource[]>(this.apiUrl + '/', {
      params: new HttpParams().set('includeDisabled', includeDisabled),
    });
  }

  createSource(request: ICreateSourceRequest): Observable<IConnectedSource> {
    return this.http.post<IConnectedSource>(this.apiUrl + '/', request);
  }

  sync(sourceId: string, request: IImportRequest): Observable<ISourceSyncResult> {
    return this.http.post<ISourceSyncResult>(`${this.apiUrl}/${sourceId}/sync`, request);
  }

  reindex(sourceId: string): Observable<ISourceSyncResult> {
    return this.http.post<ISourceSyncResult>(`${this.apiUrl}/${sourceId}/reindex`, {});
  }

  pause(sourceId: string): Observable<IConnectedSource> {
    return this.http.post<IConnectedSource>(`${this.apiUrl}/${sourceId}/pause`, {});
  }

  resume(sourceId: string): Observable<IConnectedSource> {
    return this.http.post<IConnectedSource>(`${this.apiUrl}/${sourceId}/resume`, {});
  }

  revoke(sourceId: string): Observable<IConnectedSource> {
    return this.http.post<IConnectedSource>(`${this.apiUrl}/${sourceId}/revoke`, {});
  }

  search(request: ISourceSearchRequest): Observable<ISourceSearchResult> {
    return this.http.post<ISourceSearchResult>(`${this.apiUrl}/search`, request);
  }

  extractions(projectKey: string, includeArchived: boolean): Observable<ISourceExtraction[]> {
    return this.http.get<ISourceExtraction[]>(`${this.apiUrl}/extractions`, {
      params: new HttpParams()
        .set('projectKey', projectKey || '')
        .set('includeArchived', includeArchived),
    });
  }

  updateExtraction(id: string, extraction: Partial<ISourceExtraction>): Observable<ISourceExtraction> {
    return this.http.patch<ISourceExtraction>(`${this.apiUrl}/extractions/${id}`, extraction);
  }

  archiveExtraction(id: string): Observable<ISourceExtraction> {
    return this.http.post<ISourceExtraction>(`${this.apiUrl}/extractions/${id}/archive`, {});
  }

  deleteExtraction(id: string): Observable<void> {
    return this.http.delete<void>(`${this.apiUrl}/extractions/${id}`);
  }

  auditLogs(sourceId?: string): Observable<ISourceAuditLog[]> {
    let params = new HttpParams();
    if (sourceId) {
      params = params.set('sourceId', sourceId);
    }
    return this.http.get<ISourceAuditLog[]>(`${this.apiUrl}/audit-logs`, { params });
  }
}
