import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IResearchResponse, IResearchStatus } from '../models/research.model.interface';

@Injectable({ providedIn: 'root' })
export class ResearchService {
  private readonly apiUrl = '/api/v1/research';

  constructor(private http: HttpClient) {}

  status(): Observable<IResearchStatus> {
    return this.http.get<IResearchStatus>(`${this.apiUrl}/status`);
  }

  search(query: string, limit = 5): Observable<IResearchResponse> {
    return this.http.post<IResearchResponse>(`${this.apiUrl}/search`, { query, limit });
  }
}
