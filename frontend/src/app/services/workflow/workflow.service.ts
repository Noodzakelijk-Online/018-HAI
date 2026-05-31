import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IWorkflowService } from '../workflow.service.interface';
import {
  IWorkflowChecklistUpdateRequest,
  IWorkflowIntakeRequest,
  IWorkflowItem,
  IWorkflowOverview,
  IWorkflowRecord,
  IWorkflowTransitionRequest,
} from '../../models/workflow.model.interface';

@Injectable({
  providedIn: 'root',
})
export class WorkflowService implements IWorkflowService {
  private apiUrl = '/api/v1/workflow';

  constructor(private http: HttpClient) {}

  overview(): Observable<IWorkflowOverview> {
    return this.http.get<IWorkflowOverview>(`${this.apiUrl}/overview`);
  }

  items(includeArchived: boolean): Observable<IWorkflowItem[]> {
    return this.http.get<IWorkflowItem[]>(`${this.apiUrl}/`, {
      params: new HttpParams().set('includeArchived', includeArchived),
    });
  }

  intake(request: IWorkflowIntakeRequest): Observable<IWorkflowRecord> {
    return this.http.post<IWorkflowRecord>(`${this.apiUrl}/intake`, request);
  }

  get(id: string): Observable<IWorkflowRecord> {
    return this.http.get<IWorkflowRecord>(`${this.apiUrl}/${id}`);
  }

  transition(id: string, request: IWorkflowTransitionRequest): Observable<IWorkflowRecord> {
    return this.http.post<IWorkflowRecord>(`${this.apiUrl}/${id}/transition`, request);
  }

  updateChecklistItem(
    id: string,
    itemId: string,
    request: IWorkflowChecklistUpdateRequest
  ): Observable<IWorkflowRecord> {
    return this.http.patch<IWorkflowRecord>(`${this.apiUrl}/${id}/checklist/${itemId}`, request);
  }
}
