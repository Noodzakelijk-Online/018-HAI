import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IWorkflowService } from '../workflow.service.interface';
import {
  IWorkflowApprovalResolutionRequest,
  IWorkflowChecklistUpdateRequest,
  IWorkflowClaimRecoverySummary,
  IWorkflowDashboard,
  IWorkflowIntakeRequest,
  IWorkflowInterruptedExecutionResolutionRequest,
  IWorkflowItem,
  IWorkflowOpenLoopRunSummary,
  IWorkflowOverview,
  IWorkflowProposalResolutionRequest,
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

  resolveInterruptedExecution(
    id: string,
    request: IWorkflowInterruptedExecutionResolutionRequest
  ): Observable<IWorkflowRecord> {
    return this.http.post<IWorkflowRecord>(`${this.apiUrl}/${id}/interruption/resolve`, request);
  }

  resolveProposal(
    id: string,
    proposalId: string,
    request: IWorkflowProposalResolutionRequest
  ): Observable<IWorkflowRecord> {
    return this.http.post<IWorkflowRecord>(`${this.apiUrl}/${id}/proposals/${proposalId}/resolve`, request);
  }

  updateChecklistItem(
    id: string,
    itemId: string,
    request: IWorkflowChecklistUpdateRequest
  ): Observable<IWorkflowRecord> {
    return this.http.patch<IWorkflowRecord>(`${this.apiUrl}/${id}/checklist/${itemId}`, request);
  }

  recoverStaleClaims(request: IWorkflowRunDueRequest): Observable<IWorkflowClaimRecoverySummary> {
    return this.http.post<IWorkflowClaimRecoverySummary>(`${this.apiUrl}/recover-stale`, request);
  }

  runDue(request: IWorkflowRunDueRequest): Observable<IWorkflowRunSummary> {
    return this.http.post<IWorkflowRunSummary>(`${this.apiUrl}/run-due`, request);
  }

  runDueOpenLoops(request: IWorkflowRunDueRequest): Observable<IWorkflowOpenLoopRunSummary> {
    return this.http.post<IWorkflowOpenLoopRunSummary>(`${this.apiUrl}/open-loops/run-due`, request);
  }
}
