import { HttpClient, HttpParams } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import {
  IClaimAssessment,
  IClaimLifecycle,
  IClaimReviewQueue,
  ICorrectClaimRequest,
  IKnowledgeClaim,
} from '../models/knowledge-claim.model.interface'

@Injectable({ providedIn: 'root' })
export class KnowledgeClaimService {
  private readonly apiUrl = '/api/v1/knowledge/claims'

  constructor(private http: HttpClient) {}

  reviewQueue(workspaceId: string): Observable<IClaimReviewQueue> {
    return this.http.get<IClaimReviewQueue>(`${this.apiUrl}/review-queue`, {
      params: new HttpParams().set('workspaceId', workspaceId),
    })
  }

  lifecycle(workspaceId: string, claimId: string): Observable<IClaimLifecycle> {
    return this.http.get<IClaimLifecycle>(
      `${this.apiUrl}/${encodeURIComponent(claimId)}/lifecycle`,
      { params: new HttpParams().set('workspaceId', workspaceId) },
    )
  }

  assessment(workspaceId: string, claimId: string): Observable<IClaimAssessment> {
    return this.http.get<IClaimAssessment>(
      `${this.apiUrl}/${encodeURIComponent(claimId)}/assessment`,
      { params: new HttpParams().set('workspaceId', workspaceId) },
    )
  }

  correct(claimId: string, request: ICorrectClaimRequest): Observable<IKnowledgeClaim> {
    return this.http.post<IKnowledgeClaim>(
      `${this.apiUrl}/${encodeURIComponent(claimId)}/corrections`,
      request,
    )
  }
}
