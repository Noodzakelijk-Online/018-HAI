import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { ITrivyStatus } from '../models/trivy.model.interface'

@Injectable({ providedIn: 'root' })
export class TrivyService {
  constructor(private http: HttpClient) {}

  status(): Observable<ITrivyStatus> {
    return this.http.get<ITrivyStatus>('/api/v1/trivy/status')
  }
}
