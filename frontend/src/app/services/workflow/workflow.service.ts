import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IWorkflowService } from '../workflow.service.interface';
import {
  IWorkflowApprovalResolutionRequest,
  IWorkflowChecklistUpdateRequest,
  IWorkflowDashboard,
  IWorkflowIntakeRequest,
  IWorkflowItem,
  IWorkflowOverview,
  IWorkflowRecord,
  IWorkflowRunDueRequest,
  IWorkflowRunSummary,
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

  dashboard(): Observable<IWorkflowDashboard> {
    return this.http.get<IWorkflowDashboard>(`${this.apiUrl}/dashboard`);
  }

  items(includeArchived: boolean): Observable<IWorkflowItem[]> {
    return this.http.get<IWorkflowItem[]>(`${this.apiUrl}/`, {
      params: new HttpParams().set('includeArchived', includeArchived),
    });
  }

  approvals(): Observable<IWorkflowItem[]> {
    return this.http.get<IWorkflowItem[]>(`${this.apiUrl}/approvals`);
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

  resolveApproval(id: string, request: IWorkflowApprovalResolutionRequest): Observable<IWorkflowRecord> {
    return this.http.post<IWorkflowRecord>(`${this.apiUrl}/${id}/approval`, request);
  }

  updateChecklistItem(
    id: string,
    itemId: string,
    request: IWorkflowChecklistUpdateRequest
  ): Observable<IWorkflowRecord> {
    return this.http.patch<IWorkflowRecord>(`${this.apiUrl}/${id}/checklist/${itemId}`, request);
  }

  runDue(request: IWorkflowRunDueRequest): Observable<IWorkflowRunSummary> {
    return this.http.post<IWorkflowRunSummary>(`${this.apiUrl}/run-due`, request);
  }
}
