import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ILLMPolicyService } from '../llm-policy.service.interface';
import {
  ILLMPolicy,
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

  routeTask(request: ILLMRouteRequest): Observable<ILLMRouteDecision> {
    return this.http.post<ILLMRouteDecision>(`${this.apiUrl}/route`, request);
  }

  getLogs(): Observable<ILLMRouteDecision[]> {
    return this.http.get<ILLMRouteDecision[]>(`${this.apiUrl}/logs`);
  }
}
