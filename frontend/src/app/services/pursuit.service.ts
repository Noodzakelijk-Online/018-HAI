import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { map, Observable } from 'rxjs';
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
  IPursuitDelegationPackage,
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
    return this.http.get<IPursuitDashboard>(`${this.apiUrl}/dashboard`).pipe(
      map((dashboard) => this.normalizeDashboard(dashboard)),
    );
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
    return this.http.get<IPursuitDetail>(`${this.apiUrl}/${id}`).pipe(map((detail) => this.normalizeDetail(detail)));
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

  reopen(id: string, note: string = ''): Observable<IPursuit> {
    return this.http.post<IPursuit>(`${this.apiUrl}/${id}/reopen`, { note });
  }

  refreshSummary(id: string, actor: string = 'hai'): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/summary`, { actor }).pipe(map((detail) => this.normalizeDetail(detail)));
  }

  review(id: string, request: IPursuitReviewRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/review`, request).pipe(map((detail) => this.normalizeDetail(detail)));
  }

  intake(id: string, request: IPursuitIntakeRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/intake`, request).pipe(map((detail) => this.normalizeDetail(detail)));
  }

  routeIntake(request: IPursuitIntakeRequest): Observable<IPursuitRoutedIntakeResult> {
    return this.http.post<IPursuitRoutedIntakeResult>(`${this.apiUrl}/intake`, request).pipe(
      map((result) => {
        const source = result || ({} as IPursuitRoutedIntakeResult);
        return {
          ...source,
          matches: source.matches || [],
          detail: source.detail ? this.normalizeDetail(source.detail) : undefined,
        };
      }),
    );
  }

  plan(id: string, request: IPursuitPlanRequest = {}): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/plan`, request).pipe(map((detail) => this.normalizeDetail(detail)));
  }

  resolveDecision(id: string, request: IPursuitDecisionResolutionRequest): Observable<IPursuitDetail> {
    return this.http.post<IPursuitDetail>(`${this.apiUrl}/${id}/decisions/resolve`, request).pipe(map((detail) => this.normalizeDetail(detail)));
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

  delegationPackage(id: string): Observable<IPursuitDelegationPackage> {
    return this.http.get<IPursuitDelegationPackage>(`${this.apiUrl}/${id}/delegation`);
  }

  private normalizeDashboard(dashboard: IPursuitDashboard | null | undefined): IPursuitDashboard {
    const source = dashboard || ({} as IPursuitDashboard);
    return {
      ...source,
      counts: source.counts || {},
      decisionQueue: source.decisionQueue || [],
      needsRobert: source.needsRobert || [],
      vaReady: source.vaReady || [],
      systemReady: source.systemReady || [],
      blocked: source.blocked || [],
      stale: source.stale || [],
      reviewDue: source.reviewDue || [],
      planningNeeded: source.planningNeeded || [],
      recentlyChanged: source.recentlyChanged || [],
      highRisk: source.highRisk || [],
      completionCandidates: source.completionCandidates || [],
    };
  }

  private normalizeDetail(detail: IPursuitDetail | null | undefined): IPursuitDetail {
    const source = detail || ({} as IPursuitDetail);
    return {
      ...source,
      links: source.links || [],
      activity: source.activity || [],
      workflows: source.workflows || [],
      checklistItems: source.checklistItems || [],
      openLoops: source.openLoops || [],
      proposals: source.proposals || [],
      qualityGates: source.qualityGates || [],
      decisions: source.decisions || [],
      decisionQueue: source.decisionQueue || [],
      transitions: source.transitions || [],
      sourceLinks: source.sourceLinks || [],
      events: source.events || [],
      timeline: source.timeline || [],
      evidence: source.evidence || [],
      memories: source.memories || [],
      conversations: source.conversations || [],
      ambientOpportunities: source.ambientOpportunities || [],
      taskRuns: source.taskRuns || [],
      taskAttempts: source.taskAttempts || [],
      verificationRuns: source.verificationRuns || [],
      verificationClaims: source.verificationClaims || [],
      verificationEvidence: source.verificationEvidence || [],
      automations: source.automations || [],
      runtimeAttempts: source.runtimeAttempts || [],
      sourceItems: source.sourceItems || [],
      sourceExtractions: source.sourceExtractions || [],
      nextActions: source.nextActions || [],
      blockers: source.blockers || [],
      approvalItems: source.approvalItems || [],
      actionQueues: source.actionQueues || { needsRobert: [], vaReady: [], systemReady: [], waiting: [] },
    };
  }
}
