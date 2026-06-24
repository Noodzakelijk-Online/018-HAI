import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { forkJoin, Observable } from 'rxjs';
import {
  IAgentRuntimeHealth,
  IAgentRuntimeInfo,
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
    return forkJoin({
      runtimes: this.http.get<IAgentRuntimeInfo[]>(`${this.apiUrl}/`),
      health: this.http.get<IAgentRuntimeHealth[]>(`${this.apiUrl}/health`),
    });
  }
}
