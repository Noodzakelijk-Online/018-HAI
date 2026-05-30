import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ITaskPlanService } from '../task-plan.service.interface';
import {
  ICompletionPlan,
  IReviewQueueItem,
  ITaskPlanRequest,
} from '../../models/task-plan.model.interface';

@Injectable({
  providedIn: 'root',
})
export class TaskPlanService implements ITaskPlanService {
  private apiUrl = '/api/v1/task';

  constructor(private http: HttpClient) {}

  plan(request: ITaskPlanRequest): Observable<ICompletionPlan> {
    return this.http.post<ICompletionPlan>(`${this.apiUrl}/plan`, request);
  }

  run(request: ITaskPlanRequest): Observable<ICompletionPlan> {
    return this.http.post<ICompletionPlan>(`${this.apiUrl}/run`, request);
  }

  logs(): Observable<ICompletionPlan[]> {
    return this.http.get<ICompletionPlan[]>(`${this.apiUrl}/logs`);
  }

  reviewQueue(): Observable<IReviewQueueItem[]> {
    return this.http.get<IReviewQueueItem[]>(`${this.apiUrl}/review-queue`);
  }
}
