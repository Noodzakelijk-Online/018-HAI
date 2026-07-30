import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IMLflowStatus } from '../models/mlflow.model.interface'

@Injectable({ providedIn: 'root' })
export class MLflowService {
  constructor(private http: HttpClient) {}

  status(): Observable<IMLflowStatus> {
    return this.http.get<IMLflowStatus>('/api/v1/mlflow/status')
  }
}
