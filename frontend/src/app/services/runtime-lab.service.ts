import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import {
  IRuntimeAttempt,
  IRuntimeCapabilityOverview,
  IRuntimeParityInventory,
  IRuntimeParityOverview,
  IRuntimeProbe,
  IRuntimeSummary,
} from '../models/runtime-lab.model.interface'

@Injectable({ providedIn: 'root' })
export class RuntimeLabService {
  private readonly apiUrl = '/api/v1/runtime-lab'

  constructor(private http: HttpClient) {}

  overview(): Observable<{ runtimes: IRuntimeSummary[] }> {
    return this.http.get<{ runtimes: IRuntimeSummary[] }>(`${this.apiUrl}/overview`)
  }

  featureParity(): Observable<IRuntimeParityOverview> {
    return this.http.get<IRuntimeParityOverview>(`${this.apiUrl}/feature-parity`)
  }

  capabilities(): Observable<IRuntimeCapabilityOverview> {
    return this.http.get<IRuntimeCapabilityOverview>(`${this.apiUrl}/capabilities`)
  }

  runtimeFeatureParity(runtimeId: string): Observable<IRuntimeParityInventory> {
    return this.http.get<IRuntimeParityInventory>(`${this.apiUrl}/${runtimeId}/feature-parity`)
  }

  probe(runtimeId: string): Observable<IRuntimeProbe> {
    return this.http.post<IRuntimeProbe>(`${this.apiUrl}/${runtimeId}/probe`, {})
  }

  selfTest(runtimeId: string): Observable<IRuntimeAttempt> {
    return this.http.post<IRuntimeAttempt>(`${this.apiUrl}/${runtimeId}/self-test`, {})
  }

  attempts(runtimeId: string): Observable<{ attempts: IRuntimeAttempt[] }> {
    return this.http.get<{ attempts: IRuntimeAttempt[] }>(`${this.apiUrl}/${runtimeId}/attempts`)
  }
}
