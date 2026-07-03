import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAssistantCommandRequest,
  IAssistantCommandResult,
} from '../models/assistant-command.model.interface';

@Injectable({ providedIn: 'root' })
export class AssistantCommandService {
  private readonly apiUrl = '/api/v1/assistant';

  constructor(private http: HttpClient) {}

  command(request: IAssistantCommandRequest): Observable<IAssistantCommandResult> {
    return this.http.post<IAssistantCommandResult>(`${this.apiUrl}/command`, request);
  }

  logs(): Observable<IAssistantCommandResult[]> {
    return this.http.get<IAssistantCommandResult[]>(`${this.apiUrl}/logs`);
  }
}
