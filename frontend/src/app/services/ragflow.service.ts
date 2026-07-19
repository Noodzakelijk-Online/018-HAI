import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IRAGFlowStatus } from '../models/ragflow.model.interface'

@Injectable({ providedIn: 'root' })
export class RAGFlowService {
  constructor(private http: HttpClient) {}

  status(): Observable<IRAGFlowStatus> {
    return this.http.get<IRAGFlowStatus>('/api/v1/ragflow/status')
  }
}
