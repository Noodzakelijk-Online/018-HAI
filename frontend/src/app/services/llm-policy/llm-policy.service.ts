import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ILLMPolicyService } from '../llm-policy.service.interface';
import {
  ILLMPolicy,
  ILLMModelMaintenanceResult,
  ILLMModelMaintenanceRun,
  ILLMProviderProbe,
  ILLMRouteDecision,
  ILLMRouteRequest,
} from '../../models/llm-policy.model.interface';

@Injectable({
  providedIn: 'root',
})
export class LLMPolicyService implements ILLMPolicyService {
  private apiUrl = '/api/v1/llm';

  constructor(private http: HttpClient) {}

  getPolicy(): Observable<ILLMPolicy> {
    return this.http.get<ILLMPolicy>(`${this.apiUrl}/policy`);
  }

  probeProviders(): Observable<ILLMProviderProbe[]> {
    return this.http.get<ILLMProviderProbe[]>(`${this.apiUrl}/probes`);
  }

  getProbeHistory(limit: number = 30): Observable<ILLMProviderProbe[]> {
    return this.http.get<ILLMProviderProbe[]>(`${this.apiUrl}/probes/history`, {
      params: { limit: String(limit) },
    });
  }

  getModelMaintenanceHistory(limit: number = 30): Observable<ILLMModelMaintenanceResult[]> {
    return this.http.get<ILLMModelMaintenanceResult[]>(`${this.apiUrl}/model-maintenance`, {
      params: { limit: String(limit) },
    });
  }

  runDueModelMaintenance(): Observable<ILLMModelMaintenanceRun> {
    return this.http.post<ILLMModelMaintenanceRun>(`${this.apiUrl}/model-maintenance/run`, {});
  }

  routeTask(request: ILLMRouteRequest): Observable<ILLMRouteDecision> {
    return this.http.post<ILLMRouteDecision>(`${this.apiUrl}/route`, request);
  }

  getLogs(): Observable<ILLMRouteDecision[]> {
    return this.http.get<ILLMRouteDecision[]>(`${this.apiUrl}/logs`);
  }
}
