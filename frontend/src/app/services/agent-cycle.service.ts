import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAgentCycleRunRequest,
  IAgentCycleRunResult,
} from '../models/agent-cycle.model.interface';

@Injectable({ providedIn: 'root' })
export class AgentCycleService {
  private readonly apiUrl = '/api/v1/agent-cycle';

  constructor(private http: HttpClient) {}

  run(request: IAgentCycleRunRequest): Observable<IAgentCycleRunResult> {
    return this.http.post<IAgentCycleRunResult>(`${this.apiUrl}/run`, request);
  }
}
