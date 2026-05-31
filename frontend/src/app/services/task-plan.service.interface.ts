import { Observable } from 'rxjs';
import {
  IApprovalDecision,
  ICompletionPlan,
  IReviewQueueItem,
  IReviewResolutionResult,
  ITaskPlanRequest,
} from '../models/task-plan.model.interface';

export interface ITaskPlanService {
  plan(request: ITaskPlanRequest): Observable<ICompletionPlan>;
  run(request: ITaskPlanRequest): Observable<ICompletionPlan>;
  logs(): Observable<ICompletionPlan[]>;
  reviewQueue(): Observable<IReviewQueueItem[]>;
  resolveReviewItem(id: string, decision: IApprovalDecision): Observable<IReviewResolutionResult>;
}
