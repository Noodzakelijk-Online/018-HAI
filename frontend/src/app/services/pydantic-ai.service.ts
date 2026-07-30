import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IPydanticAIResponse } from '../models/pydantic-ai.model.interface'

@Injectable({ providedIn: 'root' })
export class PydanticAIService {
  private readonly apiUrl = '/api/v1/pydantic-ai'

  constructor(private http: HttpClient) {}

  propose(request: string, successCriteria: string[]): Observable<IPydanticAIResponse> {
    return this.http.post<IPydanticAIResponse>(`${this.apiUrl}/proposals`, { request, successCriteria })
  }
}
