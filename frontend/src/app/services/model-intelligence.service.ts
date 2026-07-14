import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import {
  IBenchmarkResult,
  IHardwareResponse,
  IModelIntelligenceOverview,
  IModelProfile,
  IModelTelemetry,
  IOperationBudget,
  IPowerPolicy,
} from '../models/model-intelligence.model.interface'

@Injectable({ providedIn: 'root' })
export class ModelIntelligenceService {
  private readonly apiUrl = '/api/v1'

  constructor(private http: HttpClient) {}

  overview(): Observable<IModelIntelligenceOverview> {
    return this.http.get<IModelIntelligenceOverview>(`${this.apiUrl}/model-intelligence/overview`)
  }

  profiles(): Observable<{ profiles: IModelProfile[] }> {
    return this.http.get<{ profiles: IModelProfile[] }>(`${this.apiUrl}/model-intelligence/profiles`)
  }

  telemetry(): Observable<{ telemetry: IModelTelemetry[] }> {
    return this.http.get<{ telemetry: IModelTelemetry[] }>(`${this.apiUrl}/model-intelligence/telemetry`)
  }

  benchmark(providerId: string, modelId: string): Observable<IBenchmarkResult> {
    return this.http.post<IBenchmarkResult>(
      `${this.apiUrl}/model-intelligence/profiles/${providerId}/${modelId}/benchmark`,
      {}
    )
  }

  tokenBudgets(): Observable<IOperationBudget> {
    return this.http.get<IOperationBudget>(`${this.apiUrl}/model-intelligence/token-budgets`)
  }

  hardware(): Observable<IHardwareResponse> {
    return this.http.get<IHardwareResponse>(`${this.apiUrl}/hardware/profile`)
  }

  detectHardware(): Observable<IHardwareResponse> {
    return this.http.post<IHardwareResponse>(`${this.apiUrl}/hardware/detect`, {})
  }

  powerPolicy(): Observable<IPowerPolicy> {
    return this.http.get<IPowerPolicy>(`${this.apiUrl}/power/policy`)
  }
}
