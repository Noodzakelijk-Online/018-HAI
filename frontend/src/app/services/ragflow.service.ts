import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IRAGFlowProbeResult, IRAGFlowResponse, IRAGFlowStatus } from '../models/ragflow.model.interface'

@Injectable({ providedIn: 'root' })
export class RAGFlowService {
  private readonly apiUrl = '/api/v1/ragflow'

  constructor(private http: HttpClient) {}

  status(): Observable<IRAGFlowStatus> {
    return this.http.get<IRAGFlowStatus>(`${this.apiUrl}/status`)
  }

  probe(): Observable<IRAGFlowProbeResult> {
    return this.http.post<IRAGFlowProbeResult>(`${this.apiUrl}/probe`, {})
  }

  retrieve(query: string, limit = 5): Observable<IRAGFlowResponse> {
    return this.http.post<IRAGFlowResponse>(`${this.apiUrl}/retrieve`, { query, limit })
  }
}
