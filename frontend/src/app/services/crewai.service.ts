import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ICrewAIResponse } from '../models/crewai.model.interface';

@Injectable({ providedIn: 'root' })
export class CrewAIService {
  private readonly apiUrl = '/api/v1/crewai';

  constructor(private http: HttpClient) {}

  propose(request: string, successCriteria: string[]): Observable<ICrewAIResponse> {
    return this.http.post<ICrewAIResponse>(`${this.apiUrl}/proposals`, { request, successCriteria });
  }
}
