import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ITaskPlanService } from '../task-plan.service.interface';
import {
  IApprovalDecision,
	IApprovedReviewReconciliationRequest,
	IApprovedReviewReconciliationResult,
  ICompletionPlan,
  IReviewQueueItem,
  IReviewResolutionResult,
  ITaskPlanRequest,
} from '../../models/task-plan.model.interface';

@Injectable({
  providedIn: 'root',
})
export class TaskPlanService implements ITaskPlanService {
  private apiUrl = '/api/v1/task';

  constructor(private http: HttpClient) {}

  plan(request: ITaskPlanRequest): Observable<ICompletionPlan> {
	return this.http.post<ICompletionPlan>(`${this.apiUrl}/plan`, this.withOperationIdentity(request));
  }

  run(request: ITaskPlanRequest): Observable<ICompletionPlan> {
	return this.http.post<ICompletionPlan>(`${this.apiUrl}/run`, this.withOperationIdentity(request));
  }

	private withOperationIdentity(request: ITaskPlanRequest): ITaskPlanRequest {
		if (request.idempotencyKey?.trim()) {
			return { ...request, idempotencyKey: request.idempotencyKey.trim() };
		}
		return { ...request, idempotencyKey: crypto.randomUUID() };
	}

  logs(): Observable<ICompletionPlan[]> {
    return this.http.get<ICompletionPlan[]>(`${this.apiUrl}/logs`);
  }

  reviewQueue(): Observable<IReviewQueueItem[]> {
    return this.http.get<IReviewQueueItem[]>(`${this.apiUrl}/review-queue`);
  }

  resolveReviewItem(id: string, decision: IApprovalDecision): Observable<IReviewResolutionResult> {
    return this.http.post<IReviewResolutionResult>(`${this.apiUrl}/review-queue/${id}/resolve`, decision);
  }

	reconcileApprovedReviews(request: IApprovedReviewReconciliationRequest): Observable<IApprovedReviewReconciliationResult> {
		return this.http.post<IApprovedReviewReconciliationResult>(`${this.apiUrl}/review-queue/reconcile`, request);
	}
}
