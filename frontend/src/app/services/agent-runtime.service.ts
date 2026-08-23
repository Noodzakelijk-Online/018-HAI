import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAgentRuntimeHealth,
  IAgentRuntimeInfo,
  IAgentRuntimeSkill,
  IAgentRuntimeStopResult,
} from '../models/agent-runtime.model.interface';

@Injectable({
  providedIn: 'root',
})
export class AgentRuntimeService {
  private readonly apiUrl = '/api/v1/agent-runtimes';

  constructor(private http: HttpClient) {}

  overview(): Observable<{
    runtimes: IAgentRuntimeInfo[];
    health: IAgentRuntimeHealth[];
  }> {
    return this.http.get<{
      runtimes: IAgentRuntimeInfo[];
      health: IAgentRuntimeHealth[];
    }>(`${this.apiUrl}/overview`);
  }

  openClawEcosystem(): Observable<IAgentRuntimeInfo> {
    return this.http.get<IAgentRuntimeInfo>(`${this.apiUrl}/openclaw/ecosystem`);
  }

  skills(runtimeId: string): Observable<IAgentRuntimeSkill[]> {
    return this.http.get<IAgentRuntimeSkill[]>(`${this.apiUrl}/${runtimeId}/skills`);
  }

  stopTask(runtimeId: string, taskId: string): Observable<IAgentRuntimeStopResult> {
    return this.http.post<IAgentRuntimeStopResult>(`${this.apiUrl}/${runtimeId}/tasks/${taskId}/stop`, null);
  }

  setOpenClawEcosystemPath(ecosystemPath: string): Observable<IAgentRuntimeInfo> {
    return this.http.patch<IAgentRuntimeInfo>(`${this.apiUrl}/openclaw/ecosystem`, { ecosystemPath });
  }

  uploadOpenClawEcosystem(file: File): Observable<IAgentRuntimeInfo> {
    const form = new FormData();
    form.append('ecosystem', file, file.name);
    return this.http.post<IAgentRuntimeInfo>(`${this.apiUrl}/openclaw/ecosystem/upload`, form);
  }

  refreshOpenClawEcosystem(): Observable<IAgentRuntimeInfo> {
    return this.http.post<IAgentRuntimeInfo>(`${this.apiUrl}/openclaw/ecosystem/refresh`, null);
  }
}
