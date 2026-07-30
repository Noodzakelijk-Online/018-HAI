import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { ILangfuseExportResult, ILangfuseProbeResult, ILangfuseStatus } from '../models/langfuse.model.interface'

@Injectable({ providedIn: 'root' })
export class LangfuseService {
  constructor(private http: HttpClient) {}

  status(): Observable<ILangfuseStatus> {
    return this.http.get<ILangfuseStatus>('/api/v1/langfuse/status')
  }

  probe(): Observable<ILangfuseProbeResult> {
    return this.http.post<ILangfuseProbeResult>('/api/v1/langfuse/probe', {})
  }

  exportOperationalSnapshot(): Observable<ILangfuseExportResult> {
    return this.http.post<ILangfuseExportResult>('/api/v1/langfuse/export/operational-snapshot', {})
  }
}
