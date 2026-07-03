import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import {
  IPursuit,
  IPursuitActivity,
  IPursuitAction,
  IPursuitApprovalOverview,
  IPursuitBrief,
  IPursuitBlocker,
  IPursuitCreateRequest,
  IPursuitDashboard,
  IPursuitDashboardDecision,
  IPursuitDecisionResolutionRequest,
  IPursuitDetail,
  IPursuitEvidenceResolution,
  IPursuitIntakeRequest,
  IPursuitLink,
  IPursuitLinkRequest,
  IPursuitMatchCandidate,
  IPursuitMatchRequest,
  IPursuitPlanRequest,
  IPursuitReviewRequest,
  IPursuitRoutedIntakeResult,
  IPursuitUpdateRequest,
} from '../models/pursuit.model.interface';

@Injectable({
  providedIn: 'root',
})
export class PursuitService {
  private apiUrl = '/api/v1/pursuits';

  constructor(private http: HttpClient) {}

  list(includeArchived: boolean = false): Observable<IPursuit[]> {
    return this.http.get<IPursuit[]>(`${this.apiUrl}/`, {
      params: new HttpParams().set('includeArchived', includeArchived),
    });
  }

  dashboard(): Observable<IPursuitDashboard> {
    return this.http.get<IPursuitDashboard>(`${this.apiUrl}/dashboard`);
  }

  brief(): Observable<IPursuitBrief> {
    return this.http.get<IPursuitBrief>(`${this.apiUrl}/brief`);
  }

  decisions(): Observable<IPursuitDashboardDecision[]> {
    return this.http.get<IPursuitDashboardDecision[]>(`${this.apiUrl}/decisions`);
  }

  create(request: IPursuitCreateRequest): Observable<IPursuit> {
    return this.http.post<IPursuit>(`${this.apiUrl}/`, request);
  }

  get(id: string): Observable<IPursuitDetail> {
    return this.http.get<IPursuitDetail>(`${this.apiUrl}/${id}`);
  }

  resolveEvidence(id: string, uri: string): Observable<IPursuitEvidenceResolution> {
    return this.http.get<IPursuitEvidenceResolution>(`${this.apiUrl}/${id}/evidence`, {
      params: new HttpParams().set('uri', uri),
    });
  }

  update(id: string, request: IPursuitUpdateRequest): Observable<IPursuit> {
    return this.http.patch<IPursuit>(`${this.apiUrl}/${id}`, request);
  }

  archive(id: string, archived: boolean = true, actor: string = 'robert'): Observable<IPursuit> {
    return this.http.post<IPursuit>(`${this.apiUrl}/${id}/archive`, { archived, actor });
  }

  refreshSummary(id: string, actor: string = 'hai'): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/summary`, { actor });
  }

  review(id: string, request: IPursuitReviewRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/review`, request);
  }

  intake(id: string, request: IPursuitIntakeRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/intake`, request);
  }

  routeIntake(request: IPursuitIntakeRequest): Observable<IPursuitRoutedIntakeResult> {
    return this.http.post<IPursuitRoutedIntakeResult>(`${this.apiUrl}/intake`, request);
  }

  plan(id: string, request: IPursuitPlanRequest = {}): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/plan`, request);
  }

  resolveDecision(id: string, request: IPursuitDecisionResolutionRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/decisions/resolve`, request);
  }

  link(id: string, request: IPursuitLinkRequest): Observable<IPursuitLink> {
    return this.http.post<IPursuitLink>(`${this.apiUrl}/${id}/links`, request);
  }

  deleteLink(id: string, linkId: string): Observable<void> {
    return this.http.delete<void>(`${this.apiUrl}/${id}/links/${linkId}`);
  }

  match(request: IPursuitMatchRequest): Observable<IPursuitMatchCandidate[]> {
    return this.http.post<IPursuitMatchCandidate[]>(`${this.apiUrl}/match`, request);
  }

  activity(id: string): Observable<IPursuitActivity[]> {
    return this.http.get<IPursuitActivity[]>(`${this.apiUrl}/${id}/activity`);
  }

  nextActions(id: string): Observable<IPursuitAction[]> {
    return this.http.get<IPursuitAction[]>(`${this.apiUrl}/${id}/next-actions`);
  }

  blockers(id: string): Observable<IPursuitBlocker[]> {
    return this.http.get<IPursuitBlocker[]>(`${this.apiUrl}/${id}/blockers`);
  }

  approvals(id: string): Observable<IPursuitApprovalOverview> {
    return this.http.get<IPursuitApprovalOverview>(`${this.apiUrl}/${id}/approvals`);
  }
}
