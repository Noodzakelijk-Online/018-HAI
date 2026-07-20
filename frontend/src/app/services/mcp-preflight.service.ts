import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { IMCPPreflightOverview, IMCPPreflightResult } from '../models/mcp-preflight.model.interface';

@Injectable({ providedIn: 'root' })
export class MCPPreflightService {
  private readonly apiUrl = '/api/v1/mcp-preflight';

  constructor(private http: HttpClient) {}

  overview(): Observable<IMCPPreflightOverview> {
    return this.http.get<IMCPPreflightOverview>(`${this.apiUrl}/overview`);
  }

  run(serverId: string): Observable<IMCPPreflightResult> {
    return this.http.post<IMCPPreflightResult>(`${this.apiUrl}/${encodeURIComponent(serverId)}/run`, {});
  }
}
