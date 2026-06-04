import { Observable } from 'rxjs';
import {
  ILLMPolicy,
  ILLMProviderProbe,
  ILLMRouteDecision,
  ILLMRouteRequest,
} from '../models/llm-policy.model.interface';

export interface ILLMPolicyService {
  getPolicy(): Observable<ILLMPolicy>;
  probeProviders(): Observable<ILLMProviderProbe[]>;
  routeTask(request: ILLMRouteRequest): Observable<ILLMRouteDecision>;
  getLogs(): Observable<ILLMRouteDecision[]>;
}
