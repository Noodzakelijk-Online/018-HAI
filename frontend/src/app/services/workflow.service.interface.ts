import { Observable } from 'rxjs';
import {
  IWorkflowApprovalResolutionRequest,
  IWorkflowChecklistUpdateRequest,
  IWorkflowDashboard,
  IWorkflowIntakeRequest,
  IWorkflowItem,
  IWorkflowOpenLoopRunSummary,
  IWorkflowOverview,
  IWorkflowProposalResolutionRequest,
  IWorkflowRecord,
  IWorkflowRunDueRequest,
  IWorkflowRunSummary,
  IWorkflowTransitionRequest,
} from '../models/workflow.model.interface';

export interface IWorkflowService {
  overview(): Observable<IWorkflowOverview>;
  dashboard(): Observable<IWorkflowDashboard>;
  items(includeArchived: boolean): Observable<IWorkflowItem[]>;
  approvals(): Observable<IWorkflowItem[]>;
  intake(request: IWorkflowIntakeRequest): Observable<IWorkflowRecord>;
  get(id: string): Observable<IWorkflowRecord>;
  transition(id: string, request: IWorkflowTransitionRequest): Observable<IWorkflowRecord>;
  resolveApproval(id: string, request: IWorkflowApprovalResolutionRequest): Observable<IWorkflowRecord>;
  resolveProposal(id: string, proposalId: string, request: IWorkflowProposalResolutionRequest): Observable<IWorkflowRecord>;
  updateChecklistItem(id: string, itemId: string, request: IWorkflowChecklistUpdateRequest): Observable<IWorkflowRecord>;
  runDue(request: IWorkflowRunDueRequest): Observable<IWorkflowRunSummary>;
  runDueOpenLoops(request: IWorkflowRunDueRequest): Observable<IWorkflowOpenLoopRunSummary>;
}
