import { Observable } from 'rxjs';
import {
  ILLMPolicy,
  ILLMModelMaintenanceResult,
  ILLMModelMaintenanceRun,
  ILLMProviderProbe,
  ILLMRouteDecision,
  ILLMRouteRequest,
  ILLMGenerationResult,
} from '../models/llm-policy.model.interface';

export interface ILLMPolicyService {
  getPolicy(): Observable<ILLMPolicy>;
  probeProviders(): Observable<ILLMProviderProbe[]>;
  getProbeHistory(limit?: number): Observable<ILLMProviderProbe[]>;
  getModelMaintenanceHistory(limit?: number): Observable<ILLMModelMaintenanceResult[]>;
  runDueModelMaintenance(): Observable<ILLMModelMaintenanceRun>;
  routeTask(request: ILLMRouteRequest): Observable<ILLMRouteDecision>;
  getLogs(): Observable<ILLMRouteDecision[]>;
  getGenerationHistory(limit?: number): Observable<ILLMGenerationResult[]>;
}
