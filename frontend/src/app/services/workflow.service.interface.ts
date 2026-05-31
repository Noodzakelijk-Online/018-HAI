import { Observable } from 'rxjs';
import {
  IWorkflowChecklistUpdateRequest,
  IWorkflowIntakeRequest,
  IWorkflowItem,
  IWorkflowOverview,
  IWorkflowRecord,
  IWorkflowTransitionRequest,
} from '../models/workflow.model.interface';

export interface IWorkflowService {
  overview(): Observable<IWorkflowOverview>;
  items(includeArchived: boolean): Observable<IWorkflowItem[]>;
  intake(request: IWorkflowIntakeRequest): Observable<IWorkflowRecord>;
  get(id: string): Observable<IWorkflowRecord>;
  transition(id: string, request: IWorkflowTransitionRequest): Observable<IWorkflowRecord>;
  updateChecklistItem(id: string, itemId: string, request: IWorkflowChecklistUpdateRequest): Observable<IWorkflowRecord>;
}
