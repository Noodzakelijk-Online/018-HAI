import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IAnythingLLMResponse, IAnythingLLMStatus } from '../models/anythingllm.model.interface'

@Injectable({ providedIn: 'root' })
export class AnythingLLMService {
  private readonly apiUrl = '/api/v1/anythingllm'

  constructor(private http: HttpClient) {}

  status(): Observable<IAnythingLLMStatus> {
    return this.http.get<IAnythingLLMStatus>(`${this.apiUrl}/status`)
  }

  retrieve(query: string, workspaceSlug: string, limit = 5): Observable<IAnythingLLMResponse> {
    return this.http.post<IAnythingLLMResponse>(`${this.apiUrl}/retrieve`, { query, workspaceSlug, limit })
  }
}
