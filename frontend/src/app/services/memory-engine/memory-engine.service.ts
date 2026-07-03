import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAIConversationArchive,
  IAIConversationImportResult,
  ICommandDashboard,
  IMemoryEngineSearchResult,
} from '../../models/memory-engine.model.interface';
import { IMemoryEngineService } from '../memory-engine.service.interface';

@Injectable()
export class MemoryEngineService implements IMemoryEngineService {
  private readonly apiUrl = '/api/v1/memory-engine';

  constructor(private http: HttpClient) {}

  dashboard(): Observable<ICommandDashboard> {
    return this.http.get<ICommandDashboard>(`${this.apiUrl}/dashboard`);
  }

  conversations(limit = 50): Observable<IAIConversationArchive[]> {
    return this.http.get<IAIConversationArchive[]>(`${this.apiUrl}/conversations`, {
      params: { limit },
    });
  }

  importConversation(request: Record<string, unknown>): Observable<IAIConversationImportResult> {
    return this.http.post<IAIConversationImportResult>(`${this.apiUrl}/import`, request);
  }

  search(query: string, projectKey = '', limit = 12): Observable<IMemoryEngineSearchResult> {
    return this.http.post<IMemoryEngineSearchResult>(`${this.apiUrl}/search`, {
      query,
      projectKey,
      limit,
    });
  }

  deleteConversation(id: string): Observable<void> {
    return this.http.delete<void>(`${this.apiUrl}/conversations/${id}`);
  }
}
