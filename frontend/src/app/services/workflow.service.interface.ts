import { Observable } from 'rxjs';
import {
  IWorkflowApprovalResolutionRequest,
  IWorkflowChecklistUpdateRequest,
  IWorkflowClaimRecoverySummary,
  IWorkflowDashboard,
  IWorkflowFrameworkSelectionDecision,
  IWorkflowIntakeRequest,
  IWorkflowInterruptedExecutionResolutionRequest,
  IWorkflowItem,
  IWorkflowOpenLoopRunSummary,
  IWorkflowOverview,
  IWorkflowProposalResolutionRequest,
  IWorkflowRecord,
  IWorkflowReminderProposalSnapshot,
  IWorkflowReminderActivationPrepareRequest,
  IWorkflowReminderActivationRequestResult,
  IWorkflowReminderActivationDecisionRequest,
  IWorkflowReminderActivationDecisionResult,
  IWorkflowReminderActivationHistorySnapshot,
  IWorkflowReminderActivationDecisionHistory,
  IWorkflowReminderDeliveryAuthorizeRequest,
  IWorkflowReminderDeliveryAuthorizationResult,
  IWorkflowReminderDeliveryHistory,
  IWorkflowReminderDeliveryRunSummary,
  IWorkflowRunDueRequest,
  IWorkflowRunResult,
  IWorkflowRunSummary,
  IWorkflowTransitionRequest,
} from '../models/workflow.model.interface';

export interface IWorkflowService {
  overview(): Observable<IWorkflowOverview>;
  dashboard(): Observable<IWorkflowDashboard>;
  reminderProposals(horizonHours?: number, limit?: number): Observable<IWorkflowReminderProposalSnapshot>;
  prepareReminderActivation(itemId: string, request: IWorkflowReminderActivationPrepareRequest): Observable<IWorkflowReminderActivationRequestResult>;
  reminderActivationHistory(limit?: number): Observable<IWorkflowReminderActivationHistorySnapshot>;
  decideReminderActivation(requestId: string, request: IWorkflowReminderActivationDecisionRequest): Observable<IWorkflowReminderActivationDecisionResult>;
  reminderActivationDecisionHistory(requestId: string, limit?: number): Observable<IWorkflowReminderActivationDecisionHistory>;
  authorizeReminderDelivery(requestId: string, request: IWorkflowReminderDeliveryAuthorizeRequest): Observable<IWorkflowReminderDeliveryAuthorizationResult>;
  reminderDeliveryHistory(limit?: number): Observable<IWorkflowReminderDeliveryHistory>;
  runDueReminderDeliveries(request: IWorkflowRunDueRequest): Observable<IWorkflowReminderDeliveryRunSummary>;
  items(includeArchived: boolean): Observable<IWorkflowItem[]>;
  approvals(): Observable<IWorkflowItem[]>;
  intake(request: IWorkflowIntakeRequest): Observable<IWorkflowRecord>;
  get(id: string): Observable<IWorkflowRecord>;
  frameworkSelection(selectionDecisionId: string): Observable<IWorkflowFrameworkSelectionDecision | undefined>;
  transition(id: string, request: IWorkflowTransitionRequest): Observable<IWorkflowRecord>;
  resolveApproval(id: string, request: IWorkflowApprovalResolutionRequest): Observable<IWorkflowRecord>;
  resolveInterruptedExecution(id: string, request: IWorkflowInterruptedExecutionResolutionRequest): Observable<IWorkflowRecord>;
  resolveProposal(id: string, proposalId: string, request: IWorkflowProposalResolutionRequest): Observable<IWorkflowRecord>;
  updateChecklistItem(id: string, itemId: string, request: IWorkflowChecklistUpdateRequest): Observable<IWorkflowRecord>;
  recoverStaleClaims(request: IWorkflowRunDueRequest): Observable<IWorkflowClaimRecoverySummary>;
  runDue(request: IWorkflowRunDueRequest): Observable<IWorkflowRunSummary>;
  runOne(id: string): Observable<IWorkflowRunResult>;
  runDueOpenLoops(request: IWorkflowRunDueRequest): Observable<IWorkflowOpenLoopRunSummary>;
}
