import { Observable } from 'rxjs';
import {
  ILLMPolicy,
  ILLMRouteDecision,
  ILLMRouteRequest,
} from '../models/llm-policy.model.interface';

export interface ILLMPolicyService {
  getPolicy(): Observable<ILLMPolicy>;
  routeTask(request: ILLMRouteRequest): Observable<ILLMRouteDecision>;
  getLogs(): Observable<ILLMRouteDecision[]>;
}
