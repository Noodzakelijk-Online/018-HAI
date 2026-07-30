import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IOpenLITExportResult, IOpenLITStatus } from '../models/openlit.model.interface'

@Injectable({ providedIn: 'root' })
export class OpenLITService {
  constructor(private http: HttpClient) {}

  status(): Observable<IOpenLITStatus> {
    return this.http.get<IOpenLITStatus>('/api/v1/openlit/status')
  }

  exportOperationalSnapshot(): Observable<IOpenLITExportResult> {
    return this.http.post<IOpenLITExportResult>('/api/v1/openlit/export/operational-snapshot', {})
  }
}
