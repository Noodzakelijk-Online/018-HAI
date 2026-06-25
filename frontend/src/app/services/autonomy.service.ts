import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAutonomyOverview,
  IAutonomyStressResult,
} from '../models/autonomy.model.interface';

@Injectable({ providedIn: 'root' })
export class AutonomyService {
  private readonly apiUrl = '/api/v1/autonomy';

  constructor(private http: HttpClient) {}

  overview(): Observable<IAutonomyOverview> {
    return this.http.get<IAutonomyOverview>(`${this.apiUrl}/overview`);
  }

  runStressSuite(): Observable<IAutonomyStressResult> {
    return this.http.post<IAutonomyStressResult>(`${this.apiUrl}/stress`, {});
  }
}
