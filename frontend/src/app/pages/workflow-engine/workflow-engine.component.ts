import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { forkJoin } from 'rxjs';
import {
  IPursuitDetail,
  IPursuitMatchCandidate,
} from '../../models/pursuit.model.interface';
import { PursuitService } from '../../services/pursuit.service';
import {
  IWorkflowClaimRecoverySummary,
  IWorkflowDashboard,
  IWorkflowFrameworkSelectionDecision,
  IWorkflowFrameworkSelectionProvenance,
  IWorkflowItem,
  IWorkflowOpenLoopRunSummary,
  IWorkflowOverview,
  IWorkflowRecord,
  IWorkflowRunSummary,
} from '../../models/workflow.model.interface';
import { WORKFLOW_SERVICE_TOKEN } from '../../services/workflow/workflow.service.token';
import { IWorkflowService } from '../../services/workflow.service.interface';

type FrameworkProvenanceState = 'missing' | 'invalid' | 'recorded' | 'verified';

@Component({
  selector: 'app-workflow-engine',
  templateUrl: './workflow-engine.component.html',
  styleUrls: ['./workflow-engine.component.scss'],
})
export class WorkflowEngineComponent implements OnInit {
  overview?: IWorkflowOverview;
  dashboard?: IWorkflowDashboard;
  items: IWorkflowItem[] = [];
  approvalItems: IWorkflowItem[] = [];
  selected?: IWorkflowRecord;
  runSummary?: IWorkflowRunSummary;
  openLoopRunSummary?: IWorkflowOpenLoopRunSummary;
  recoverySummary?: IWorkflowClaimRecoverySummary;
  frameworkProvenance?: IWorkflowFrameworkSelectionProvenance;
  frameworkSelectionDecision?: IWorkflowFrameworkSelectionDecision;
  frameworkProvenanceState: FrameworkProvenanceState = 'missing';
  frameworkProvenanceIssues: string[] = [];
  frameworkSelectionLoading = false;
  frameworkSelectionUnavailable = false;
  pursuitMatches: IPursuitMatchCandidate[] = [];
  selectedPursuitMatch?: IPursuitMatchCandidate;
  includeArchived = false;
  loading = false;
  saving = false;
  matchingPursuits = false;
  runningAction?: 'refresh' | 'worker' | 'followups' | 'recovery';
  lastOperation?: {
    name: string;
    status: 'completed' | 'failed';
    summary: string;
    details?: string;
    at: Date;
  };
  workflowSearch = '';
  stateFilter = 'all';
  riskFilter = 'all';
  activeQueue: 'all' | 'approval' | 'ready' | 'blocked' | 'review' = 'all';
  private frameworkSelectionLookup = 0;

  intakeForm: FormGroup = this.fb.group({
    input: [
      'Email from lawyer about Vivare legal hearing tomorrow. Draft formal Dutch reply and attach evidence.',
      [Validators.required],
    ],
    projectKey: ['Vivare dispute'],
    automationId: [''],
    sourceType: ['email'],
    sourceId: ['sample-email-1'],
    sourceUri: ['local://sample/email'],
    sourceLabel: ['Sample legal email'],
    contentType: ['email'],
    sender: ['lawyer@example.test'],
    trigger: ['manual_intake'],
  });

  transitionForm: FormGroup = this.fb.group({
    targetState: ['ready', [Validators.required]],
    message: ['Move workflow to the selected state.'],
  });

  approvalForm: FormGroup = this.fb.group({
    note: ['Robert approved controlled workflow execution.'],
  });

  interruptionForm: FormGroup = this.fb.group({
    decision: ['retry', [Validators.required]],
    note: ['', [Validators.required]],
    evidenceUri: [''],
    evidenceLabel: [''],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(WORKFLOW_SERVICE_TOKEN) private workflowService: IWorkflowService,
    private pursuitService: PursuitService,
    private notification: NzNotificationService,
    private route: ActivatedRoute,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
    this.intakeForm.valueChanges.subscribe(() => {
      // A changed signal must be matched again; never link edited intake to a stale pursuit choice.
      this.selectedPursuitMatch = undefined;
      this.pursuitMatches = [];
    });
    const workflowId = this.route.snapshot.queryParamMap.get('workflowId');
    if (workflowId) {
      this.workflowService.get(workflowId).subscribe({
        next: (record) => this.applyWorkflowRecord(record),
        error: () => this.notification.error('Error', 'The linked workflow could not be opened.'),
      });
    }
  }

  refresh(showNotification = false, preserveLastOperation = false): void {
    if (this.runningAction && this.runningAction !== 'refresh') {
      return;
    }
    const blockingRefresh = !preserveLastOperation;
    this.loading = true;
    if (blockingRefresh) {
      this.runningAction = 'refresh';
    }
    forkJoin({
      overview: this.workflowService.overview(),
      dashboard: this.workflowService.dashboard(),
      items: this.workflowService.items(this.includeArchived),
      approvals: this.workflowService.approvals(),
    }).subscribe({
      next: ({ overview, dashboard, items, approvals }) => {
        this.overview = overview;
        this.dashboard = dashboard;
        this.items = items;
        this.approvalItems = approvals;
        this.loading = false;
        if (blockingRefresh) {
          this.runningAction = undefined;
        }
        if (!preserveLastOperation) {
          this.lastOperation = {
            name: 'Refresh',
            status: 'completed',
            summary: `${items.length} workflows, ${approvals.length} approvals, ${dashboard.dueOpenLoops.length} due follow-ups.`,
            at: new Date(),
          };
        }
        if (showNotification) {
          this.notification.success(
            'Workflow data refreshed',
            `${items.length} workflows, ${approvals.length} approvals, ${dashboard.dueOpenLoops.length} due follow-ups.`
          );
        }
      },
      error: () => {
        this.loading = false;
        if (blockingRefresh) {
          this.runningAction = undefined;
        }
        if (!preserveLastOperation) {
          this.lastOperation = {
            name: 'Refresh',
            status: 'failed',
            summary: 'One or more workflow panels failed to load.',
            at: new Date(),
          };
        }
        this.notification.error('Error', 'Failed to load the workflow operational chain.');
      }
    });
  }

  intake(): void {
    if (this.intakeForm.invalid) {
      return;
    }
    this.saving = true;
    if (this.selectedPursuitMatch) {
      this.pursuitService.intake(this.selectedPursuitMatch.pursuit.id, this.intakeForm.value).subscribe({
        next: (detail) => {
          this.saving = false;
          this.notification.success('Workflow linked to pursuit', 'Input became operational work under the selected pursuit.');
          this.selectNewestPursuitWorkflow(detail);
          this.refresh(false, true);
        },
        error: () => {
          this.saving = false;
          this.notification.error('Error', 'Failed to create workflow inside the selected pursuit.');
        },
      });
      return;
    }
    this.pursuitService.routeIntake(this.intakeForm.value).subscribe({
      next: (result) => {
        this.saving = false;
        this.pursuitMatches = result.matches || [];
        if (result.detail) {
          this.selectNewestPursuitWorkflow(result.detail);
        }
        if (result.mode === 'matched_existing') {
          this.notification.success('Workflow linked to pursuit', 'HAI matched this input to an existing pursuit before creating governed work.');
        } else if (result.createdCandidate) {
          this.notification.info('Pursuit candidate needs review', 'HAI recorded the unmatched input as a reviewable pursuit candidate. No workflow was created until an approver accepts it.');
        } else {
          this.notification.success('Workflow created', result.message || 'Input classified, checklist generated, and audit event recorded.');
        }
        this.refresh(false, true);
      },
      error: () => {
        this.saving = false;
        this.notification.error('Error', 'Failed to match and intake workflow input.');
      },
    });
  }

  matchPursuits(): void {
    const value = this.intakeForm.value;
    if (!String(value.input || '').trim()) {
      this.notification.error('Input required', 'Describe the signal before matching it to a pursuit.');
      return;
    }
    this.matchingPursuits = true;
    this.pursuitService.match({
      input: value.input,
      projectKey: value.projectKey,
      sourceType: value.sourceType,
      sourceId: value.sourceId,
      sourceUri: value.sourceUri,
      limit: 5,
    }).subscribe({
      next: (matches) => {
        this.pursuitMatches = matches;
        this.matchingPursuits = false;
        if (!matches.length) {
          this.selectedPursuitMatch = undefined;
          this.notification.info('No pursuit match', 'This signal can still create a standalone workflow.');
          return;
        }
        if (!this.selectedPursuitMatch && matches[0].score >= 0.7) {
          this.selectedPursuitMatch = matches[0];
        }
      },
      error: () => {
        this.matchingPursuits = false;
        this.notification.error('Error', 'Failed to retrieve pursuit matches.');
      },
    });
  }

  selectPursuitMatch(match: IPursuitMatchCandidate): void {
    this.selectedPursuitMatch = match;
  }

  clearPursuitMatch(): void {
    this.selectedPursuitMatch = undefined;
  }

  open(item: IWorkflowItem): void {
    this.workflowService.get(item.id).subscribe({
      next: (record) => this.applyWorkflowRecord(record),
      error: () => this.notification.error('Error', 'Failed to open workflow.'),
    });
  }

  transition(): void {
    if (!this.selected || this.transitionForm.invalid) {
      return;
    }
    this.workflowService.transition(this.selected.item.id, this.transitionForm.value).subscribe({
      next: (record) => {
        this.applyWorkflowRecord(record);
        this.notification.success('State updated', 'Workflow transition was validated and audited.');
        this.refresh(false, true);
      },
      error: () => this.notification.error('Blocked', 'Workflow transition was not allowed.'),
    });
  }

  resolveApproval(item: IWorkflowItem, approved: boolean): void {
    this.workflowService.resolveApproval(item.id, {
      approved,
      note: this.approvalForm.value.note,
      actor: 'operator',
    }).subscribe({
      next: (record) => {
        this.applyWorkflowRecord(record);
        this.notification.success('Approval updated', approved ? 'Workflow approved for execution.' : 'Workflow rejected and blocked.');
        this.refresh(false, true);
      },
      error: () => this.notification.error('Error', 'Failed to update workflow approval.'),
    });
  }

  resolveInterruptedExecution(): void {
    if (!this.selected || this.interruptionForm.invalid) {
      return;
    }
    const request = {
      ...this.interruptionForm.value,
      actor: 'operator',
    };
    if (request.decision === 'confirm_completed' && !request.evidenceUri?.trim()) {
      this.notification.error('Evidence required', 'Add a source URI before confirming completion.');
      return;
    }
    this.saving = true;
    this.workflowService.resolveInterruptedExecution(this.selected.item.id, request).subscribe({
      next: (record) => {
        this.applyWorkflowRecord(record);
        this.interruptionForm.reset({
          decision: 'retry',
          note: '',
          evidenceUri: '',
          evidenceLabel: '',
        });
        this.saving = false;
        this.notification.success('Interruption resolved', `Recovery decision recorded: ${request.decision}.`);
        this.refresh(false, true);
      },
      error: () => {
        this.saving = false;
        this.notification.error('Resolution blocked', 'The interrupted execution could not be resolved.');
      },
    });
  }

  runDue(): void {
    if (this.runningAction) {
      return;
    }
    this.runningAction = 'worker';
    this.workflowService.runDue({ limit: 10 }).subscribe({
      next: (summary) => {
        this.runSummary = summary;
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Run worker',
          status: 'completed',
          summary: `${summary.checked} checked, ${summary.completed} completed, ${summary.retried} retried, ${summary.blocked} blocked, ${summary.skipped} skipped.`,
          details: this.workflowRunDetails(summary),
          at: new Date(),
        };
        this.notification.success('Worker run complete', `${summary.completed} completed, ${summary.retried} retried, ${summary.blocked} blocked.`);
        this.reloadSelectedWorkflow();
        this.refresh(false, true);
      },
      error: () => {
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Run worker',
          status: 'failed',
          summary: 'Workflow worker run failed before completion.',
          at: new Date(),
        };
        this.notification.error('Error', 'Workflow worker run failed.');
      },
    });
  }

  recoverStaleClaims(): void {
    if (this.runningAction) {
      return;
    }
    this.runningAction = 'recovery';
    this.workflowService.recoverStaleClaims({ limit: 50 }).subscribe({
      next: (summary) => {
        this.recoverySummary = summary;
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Recover stale',
          status: 'completed',
          summary: `${summary.checked} checked, ${summary.workflowsBlocked} workflows blocked for review, ${summary.openLoopsReopened} follow-ups reopened, ${summary.skipped} skipped.`,
          details: this.claimRecoveryDetails(summary),
          at: new Date(),
        };
        this.notification.success(
          'Claim recovery complete',
          `${summary.workflowsBlocked} workflows blocked for review, ${summary.openLoopsReopened} follow-ups reopened.`
        );
        this.refresh(false, true);
      },
      error: () => {
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Recover stale',
          status: 'failed',
          summary: 'Stale claim recovery failed before completion.',
          at: new Date(),
        };
        this.notification.error('Error', 'Stale claim recovery failed.');
      },
    });
  }

  runDueOpenLoops(): void {
    if (this.runningAction) {
      return;
    }
    this.runningAction = 'followups';
    this.workflowService.runDueOpenLoops({ limit: 10 }).subscribe({
      next: (summary) => {
        this.openLoopRunSummary = summary;
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Run follow-ups',
          status: 'completed',
          summary: `${summary.checked} checked, ${summary.triggered} triggered, ${summary.resolved} resolved, ${summary.skipped} skipped.`,
          details: this.openLoopRunDetails(summary),
          at: new Date(),
        };
        this.notification.success('Open loops processed', `${summary.triggered} triggered, ${summary.resolved} resolved.`);
        this.refresh(false, true);
      },
      error: () => {
        this.runningAction = undefined;
        this.lastOperation = {
          name: 'Run follow-ups',
          status: 'failed',
          summary: 'Open-loop worker run failed before completion.',
          at: new Date(),
        };
        this.notification.error('Error', 'Open-loop worker run failed.');
      },
    });
  }

  resolveProposal(
    proposalId: string,
    status: 'approved' | 'changes_requested' | 'rejected',
    selectedOption?: string,
  ): void {
    if (!this.selected) {
      return;
    }
    const approved = status === 'approved';
    const noteByStatus: Record<string, string> = {
      approved: 'Proposal approved from dashboard.',
      changes_requested: 'Proposal needs changes from dashboard.',
      rejected: 'Proposal rejected from dashboard.',
    };
    this.workflowService.resolveProposal(this.selected.item.id, proposalId, {
      approved,
      status,
      selectedOption,
      note: noteByStatus[status],
      actor: 'operator',
    }).subscribe({
      next: (record) => {
        this.applyWorkflowRecord(record);
        this.notification.success('Proposal updated', noteByStatus[status]);
        this.refresh();
      },
      error: () => this.notification.error('Error', 'Failed to update proposal.'),
    });
  }

  markChecklist(itemId: string, status: string): void {
    if (!this.selected) {
      return;
    }
    this.workflowService.updateChecklistItem(this.selected.item.id, itemId, { status }).subscribe({
      next: (record) => this.applyWorkflowRecord(record),
      error: () => this.notification.error('Error', 'Failed to update checklist item.'),
    });
  }

  get interruptionRequiresEvidence(): boolean {
    return this.interruptionForm.get('decision')?.value === 'confirm_completed';
  }

  applyWorkflowRecord(record: IWorkflowRecord): void {
    this.selected = record;
    this.frameworkSelectionDecision = undefined;
    this.frameworkSelectionLoading = false;
    this.frameworkSelectionUnavailable = false;
    this.frameworkProvenanceIssues = [];

    const selections = Array.isArray(record.frameworkSelections)
      ? record.frameworkSelections
      : [];
    if (!selections.length) {
      this.frameworkProvenance = undefined;
      this.frameworkProvenanceState = 'missing';
      this.frameworkSelectionLookup += 1;
      return;
    }

    const currentPlanId = (record.item.lastTaskPlanId || '').trim();
    const provenance = currentPlanId
      ? selections.find((selection) => selection?.taskPlanId?.trim() === currentPlanId)
      : selections[0];
    this.frameworkProvenance = provenance || selections[0];

    if (!provenance) {
      this.frameworkProvenanceState = 'invalid';
      this.frameworkProvenanceIssues = [
        'No stored framework selection matches the workflow current task plan.',
      ];
      this.frameworkSelectionLookup += 1;
      return;
    }

    this.frameworkProvenanceIssues = this.validateFrameworkProvenance(
      provenance,
      currentPlanId
    );
    if (this.frameworkProvenanceIssues.length) {
      this.frameworkProvenanceState = 'invalid';
      this.frameworkSelectionLookup += 1;
      return;
    }

    this.frameworkProvenanceState = 'recorded';
    this.frameworkSelectionLoading = true;
    const lookup = ++this.frameworkSelectionLookup;
    this.workflowService.frameworkSelection(provenance.selectionDecisionId).subscribe({
      next: (decision) => {
        if (lookup !== this.frameworkSelectionLookup) {
          return;
        }
        this.frameworkSelectionLoading = false;
        if (!decision) {
          this.frameworkSelectionUnavailable = true;
          return;
        }
        const issues = this.validateFrameworkSelectionDecision(decision, provenance);
        if (issues.length) {
          this.frameworkProvenanceState = 'invalid';
          this.frameworkProvenanceIssues = issues;
          return;
        }
        this.frameworkSelectionDecision = decision;
        this.frameworkProvenanceState = 'verified';
      },
      error: () => {
        if (lookup !== this.frameworkSelectionLookup) {
          return;
        }
        this.frameworkSelectionLoading = false;
        this.frameworkSelectionUnavailable = true;
      },
    });
  }

  get frameworkGovernanceLabel(): string {
    switch (this.frameworkProvenanceState) {
      case 'verified':
        return 'Governed selection verified';
      case 'recorded':
        return 'Selection provenance recorded';
      case 'invalid':
        return 'Framework provenance needs review';
      default:
        return 'No framework selection recorded';
    }
  }

  get frameworkGovernanceSummary(): string {
    switch (this.frameworkProvenanceState) {
      case 'verified':
        return 'The workflow provenance matches its owner-scoped Framework Registry decision.';
      case 'recorded':
        return 'Compact provenance is present, but the full registry decision is not currently confirmed.';
      case 'invalid':
        return 'Stored provenance failed validation. Do not treat this workflow as framework-governed.';
      default:
        return 'Framework-governed execution or completion cannot be confirmed from this workflow record.';
    }
  }

  get frameworkGovernanceClass(): string {
    if (this.frameworkProvenanceState === 'verified') {
      return 'governance-summary--verified';
    }
    if (this.frameworkProvenanceState === 'invalid') {
      return 'governance-summary--invalid';
    }
    return 'governance-summary--unconfirmed';
  }

  get frameworkSelectionLabel(): string {
    return this.frameworkProvenance?.selectionDecisionId
      ? `Selection ${this.shortIdentifier(this.frameworkProvenance.selectionDecisionId)}`
      : 'No selection ID';
  }

  openFrameworkRegistry(): void {
    this.router.navigate(['/framework-registry']);
  }

  copyProvenanceValue(label: string, value?: string | number): void {
    const text = String(value ?? '').trim();
    if (!text) {
      this.notification.warning('Value unavailable', `${label} is not present in this workflow record.`);
      return;
    }
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      this.notification.info(label, text);
      return;
    }
    navigator.clipboard.writeText(text).then(
      () => this.notification.success('Copied', `${label} copied.`),
      () => this.notification.info(label, text)
    );
  }

  optionLines(options?: string): string[] {
    return (options || '').split('\n').filter((option) => !!option.trim());
  }

  filteredItems(): IWorkflowItem[] {
    const query = this.workflowSearch.trim().toLowerCase();
    return this.items.filter((item) => {
      const queueMatch =
        this.activeQueue === 'all' ||
        (this.activeQueue === 'approval' && item.requiresApproval && item.approvalStatus !== 'approved') ||
        (this.activeQueue === 'ready' && item.currentState === 'ready') ||
        (this.activeQueue === 'blocked' && (item.currentState === 'blocked' || !!item.blockedReason)) ||
        (this.activeQueue === 'review' && item.recoveryStatus === 'needs_review');
      const stateMatch = this.stateFilter === 'all' || item.currentState === this.stateFilter;
      const riskMatch = this.riskFilter === 'all' || item.riskLevel === this.riskFilter;
      const textMatch =
        !query ||
        `${item.title} ${item.description || ''} ${item.projectKey || ''} ${item.nextAction || ''}`
          .toLowerCase()
          .includes(query);
      return queueMatch && stateMatch && riskMatch && textMatch;
    });
  }

  count(key: string): number {
    return this.dashboard?.counts?.[key] || 0;
  }

  queueCount(queue: 'all' | 'approval' | 'ready' | 'blocked' | 'review'): number {
    if (queue === 'all') return this.items.length;
    if (queue === 'approval') return this.approvalItems.length;
    if (queue === 'ready') return this.dashboard?.readyItems?.length || this.count('ready');
    if (queue === 'blocked') return this.dashboard?.blockedItems?.length || this.count('blocked');
    return this.count('interruptedReview');
  }

  hasWorkflowAttention(): boolean {
    return (
      this.queueCount('all') > 0 ||
      this.queueCount('approval') > 0 ||
      this.queueCount('ready') > 0 ||
      this.queueCount('blocked') > 0 ||
      this.queueCount('review') > 0 ||
      this.count('dueOpenLoops') > 0 ||
      this.count('expiredWorkflowClaims') > 0 ||
      this.count('expiredOpenLoopClaims') > 0
    );
  }

  stateOptions(): string[] {
    const states = this.overview?.states || [];
    const itemStates = this.items.map((item) => item.currentState).filter(Boolean);
    return Array.from(new Set([...states, ...itemStates])).sort();
  }

  riskOptions(): string[] {
    return Array.from(new Set(this.items.map((item) => item.riskLevel).filter(Boolean))).sort();
  }

  readable(value?: string): string {
    return (value || 'unknown').replace(/_/g, ' ');
  }

  statusClass(value?: string): string {
    const normalized = (value || '').toLowerCase();
    if (['implemented', 'completed', 'ready', 'approved', 'verified', 'source_supported'].includes(normalized)) {
      return 'status--good';
    }
    if (['partial', 'warning', 'waiting_external_input', 'needs_approval', 'needs_review'].includes(normalized)) {
      return 'status--watch';
    }
    if (['blocked', 'failed', 'rejected', 'unsupported', 'conflicting'].includes(normalized)) {
      return 'status--risk';
    }
    return 'status--neutral';
  }

  statusLabel(value?: string): string {
    const normalized = (value || '').toLowerCase();
    if (normalized === 'partial') {
      return 'in progress';
    }
    if (normalized === 'not_implemented') {
      return 'planned';
    }
    return this.readable(value);
  }

  clearFilters(): void {
    this.activeQueue = 'all';
    this.stateFilter = 'all';
    this.riskFilter = 'all';
    this.workflowSearch = '';
  }

  focusFilters(): void {
    document.getElementById('workflow-state-filter')?.focus();
  }

  focusIntake(): void {
    const element = document.getElementById('workflow-intake-input');
    if (element) {
      element.focus();
      element.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  }

  priorityClass(score?: number): string {
    if ((score || 0) >= 80) return 'priority--urgent';
    if ((score || 0) >= 50) return 'priority--medium';
    return 'priority--normal';
  }

  capabilityProgress(status?: string): number {
    if (status === 'implemented') return 100;
    if (status === 'partial') return 58;
    return 24;
  }

  isActionRunning(action: 'refresh' | 'worker' | 'followups' | 'recovery'): boolean {
    return this.runningAction === action;
  }

  anyActionRunning(): boolean {
    return !!this.runningAction;
  }

  private validateFrameworkProvenance(
    provenance: IWorkflowFrameworkSelectionProvenance,
    currentPlanId: string
  ): string[] {
    const issues: string[] = [];
    const requiredStrings: Array<[string, string | undefined]> = [
      ['Selection decision ID', provenance?.selectionDecisionId],
      ['Task plan ID', provenance?.taskPlanId],
      ['Catalog version', provenance?.catalogVersion],
      ['Selector algorithm version', provenance?.selectorAlgorithmVersion],
      ['Constitution source', provenance?.constitutionSource],
    ];
    requiredStrings.forEach(([label, value]) => {
      if (!String(value || '').trim()) {
        issues.push(`${label} is missing.`);
      }
    });

    const decisionId = String(provenance?.selectionDecisionId || '').trim();
    if (decisionId && !this.isUuid(decisionId)) {
      issues.push('Selection decision ID is not a UUID.');
    }
    if (
      currentPlanId &&
      String(provenance?.taskPlanId || '').trim() !== currentPlanId
    ) {
      issues.push('Selection provenance does not match the workflow current task plan.');
    }

    const digests: Array<[string, string | undefined]> = [
      ['Catalog digest', provenance?.catalogDigest],
      ['Preference digest', provenance?.effectivePreferenceDigest],
      ['Constitution digest', provenance?.constitutionDigest],
    ];
    digests.forEach(([label, value]) => {
      if (!this.isSha256(value)) {
        issues.push(`${label} is not a SHA-256 digest.`);
      }
    });
    if (!Number.isInteger(provenance?.constitutionVersion) || provenance.constitutionVersion < 1) {
      issues.push('Constitution version is not a positive integer.');
    }
    return Array.from(new Set(issues));
  }

  private validateFrameworkSelectionDecision(
    decision: IWorkflowFrameworkSelectionDecision,
    provenance: IWorkflowFrameworkSelectionProvenance
  ): string[] {
    const issues: string[] = [];
    const matches: Array<[string, string | number | undefined, string | number | undefined]> = [
      ['Selection decision ID', decision?.id, provenance.selectionDecisionId],
      ['Task plan ID', decision?.taskPlanId, provenance.taskPlanId],
      ['Catalog version', decision?.catalogVersion, provenance.catalogVersion],
      ['Catalog digest', decision?.catalogDigest, provenance.catalogDigest],
      [
        'Selector algorithm version',
        decision?.selectorAlgorithmVersion,
        provenance.selectorAlgorithmVersion,
      ],
      [
        'Preference digest',
        decision?.effectivePreferenceDigest,
        provenance.effectivePreferenceDigest,
      ],
      ['Constitution version', decision?.constitutionVersion, provenance.constitutionVersion],
      ['Constitution digest', decision?.constitutionDigest, provenance.constitutionDigest],
      ['Constitution source', decision?.constitutionSource, provenance.constitutionSource],
    ];
    matches.forEach(([label, actual, expected]) => {
      if (String(actual ?? '').trim() !== String(expected ?? '').trim()) {
        issues.push(`${label} does not match the persisted workflow provenance.`);
      }
    });

    if (!Array.isArray(decision?.selected) || !decision.selected.length) {
      issues.push('The Framework Registry decision has no selected frameworks.');
    } else {
      const seen = new Set<string>();
      decision.selected.forEach((framework) => {
        const id = String(framework?.id || '').trim();
        const version = String(framework?.version || '').trim();
        if (!id || !version) {
          issues.push('A selected framework is missing its ID or version.');
          return;
        }
        const key = `${id}@${version}`;
        if (seen.has(key)) {
          issues.push('The Framework Registry decision contains duplicate framework versions.');
        }
        seen.add(key);
      });
    }
    return Array.from(new Set(issues));
  }

  private isUuid(value: string): boolean {
    return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
  }

  private isSha256(value?: string): boolean {
    return /^[0-9a-f]{64}$/i.test(String(value || '').trim());
  }

  private shortIdentifier(value: string): string {
    const normalized = String(value || '').trim();
    if (normalized.length <= 18) {
      return normalized;
    }
    return `${normalized.slice(0, 8)}...${normalized.slice(-4)}`;
  }

  private reloadSelectedWorkflow(): void {
    const workflowId = this.selected?.item?.id;
    if (!workflowId) {
      return;
    }
    this.workflowService.get(workflowId).subscribe({
      next: (record) => {
        if (this.selected?.item?.id === workflowId) {
          this.applyWorkflowRecord(record);
        }
      },
      error: () => {
        this.notification.warning(
          'Workflow detail not refreshed',
          'The worker result is available, but the selected workflow detail could not be reloaded.'
        );
      },
    });
  }

  private workflowRunDetails(summary: IWorkflowRunSummary): string {
    return summary.results
      .slice(0, 5)
      .map((result) => `${result.status}: ${result.message || result.workflowId}`)
      .join(' | ');
  }

  private openLoopRunDetails(summary: IWorkflowOpenLoopRunSummary): string {
    return summary.results
      .slice(0, 5)
      .map((result) => `${result.status}: ${result.message || result.openLoopId}`)
      .join(' | ');
  }

  private claimRecoveryDetails(summary: IWorkflowClaimRecoverySummary): string {
    return summary.results
      .slice(0, 5)
      .map((result) => `${result.type}/${result.status}: ${result.message}`)
      .join(' | ');
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }

  openPursuit(id: string): void {
    this.router.navigate(['/pursuits'], { queryParams: { selected: id } });
  }

  private selectNewestPursuitWorkflow(detail: IPursuitDetail): void {
    const newest = [...(detail.workflows || [])].sort((left, right) => {
      return new Date(right.updatedAt || right.createdAt).getTime() - new Date(left.updatedAt || left.createdAt).getTime();
    })[0];
    if (newest) {
      this.open(newest);
    }
  }
}
