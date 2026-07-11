import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import {
  IBackgroundStatus,
  IEmergencyStopState,
  IEmergencyStopVerification,
  IReadiness,
  IRecoveryReport,
} from '../models/runtime-control.model.interface'

@Injectable({ providedIn: 'root' })
export class RuntimeControlService {
  private readonly apiUrl = '/api/v1'

  constructor(private http: HttpClient) {}

  status(): Observable<IBackgroundStatus> {
    return this.http.get<IBackgroundStatus>(`${this.apiUrl}/background/status`)
  }

  readiness(): Observable<IReadiness> {
    return this.http.get<IReadiness>(`${this.apiUrl}/windows-runtime/readiness`)
  }

  pause(reason: string): Observable<{ emergencyStop: IEmergencyStopState }> {
    return this.http.post<{ emergencyStop: IEmergencyStopState }>(`${this.apiUrl}/background/pause`, { reason })
  }

  resume(): Observable<{ emergencyStop: IEmergencyStopState }> {
    return this.http.post<{ emergencyStop: IEmergencyStopState }>(`${this.apiUrl}/background/resume`, {})
  }

  setMode(mode: string): Observable<{ mode: string }> {
    return this.http.patch<{ mode: string }>(`${this.apiUrl}/background/mode`, { mode })
  }

  verifyEmergencyStop(): Observable<IEmergencyStopVerification> {
    return this.http.post<IEmergencyStopVerification>(`${this.apiUrl}/windows-runtime/emergency-stop/verify`, {})
  }

  recover(): Observable<IRecoveryReport> {
    return this.http.post<IRecoveryReport>(`${this.apiUrl}/windows-runtime/recovery`, {})
  }
}
