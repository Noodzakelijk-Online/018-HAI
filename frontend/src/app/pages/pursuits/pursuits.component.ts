import { Component, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { Subscription } from 'rxjs';
import {
  IAutomationLaunchEvent,
  IAutomationRuntimeRouteTrace,
} from '../../models/automation.model.interface';
import {
  IPursuit,
  IPursuitAction,
  IPursuitAutomation,
  IPursuitBlocker,
  IPursuitDashboard,
  IPursuitDashboardDecision,
  IPursuitDelegationPackage,
  IPursuitDecision,
  IPursuitDetail,
  IPursuitEvidenceResolution,
  IPursuitListItem,
} from '../../models/pursuit.model.interface';
import { AutomationsService } from '../../services/automations/automations.service';
import { PursuitService } from '../../services/pursuit.service';
import { WorkflowService } from '../../services/workflow/workflow.service';

@Component({
  selector: 'app-pursuits',
  templateUrl: './pursuits.component.html',
  styleUrls: ['./pursuits.component.scss'],
})
export class PursuitsComponent implements OnInit, OnDestroy {
  dashboard?: IPursuitDashboard;
  pursuits: IPursuit[] = [];
  selected?: IPursuitDetail;
  loading = false;
  detailLoading = false;
  creating = false;
  intakeRunning = false;
  routedIntakeRunning = false;
  reviewing = false;
  planning = false;
  delegationLoading = false;
  resolvingDecisionId = '';
  stoppingAutomationId = '';
  resolvingEvidenceUri = '';
  inspectedEvidence?: IPursuitEvidenceResolution;
  inspectedRuntimeEvidence?: IAutomationLaunchEvent;
  inspectedAction?: IPursuitAction;
  delegationPackage?: IPursuitDelegationPackage;
  showCreate = false;
  showContextEditor = false;
  includeArchived = false;
  private requestedPursuitId = '';
  private requestedEvidenceUri = '';
  highlightedDecisionId = '';
  private routeSub?: Subscription;

  readonly domains = [
    'operations',
    'legal',
    'financial',
    'client',
    'software',
    'content',
    'personal',
    'unknown',
  ];

  readonly linkTypes = [
    'workflow',
    'memory',
    'ai_conversation',
    'ambient_opportunity',
    'source_item',
    'source_extraction',
    'verification',
    'automation',
    'agent_runtime',
  ];

  createForm: FormGroup = this.fb.group({
    title: ['', [Validators.required]],
    description: [''],
    projectKey: [''],
    domain: ['operations'],
    whyItMatters: [''],
    desiredOutcome: [''],
    currentStateSummary: [''],
    completionDefinition: [''],
  });

  contextForm: FormGroup = this.fb.group({
    description: [''],
    whyItMatters: [''],
    desiredOutcome: [''],
    currentStateSummary: [''],
    nextRecommendedAction: [''],
    completionDefinition: [''],
  });

  intakeForm: FormGroup = this.fb.group({
    input: ['New signal: describe the email, message, document, or account event here.', [Validators.required]],
    sourceType: ['manual'],
    sourceId: [''],
    sourceUri: [''],
    sourceLabel: [''],
    sender: [''],
  });

  routedIntakeForm: FormGroup = this.fb.group({
    input: ['', [Validators.required]],
    projectKey: [''],
    sourceType: ['manual'],
    sourceLabel: [''],
    sourceUri: [''],
  });

  linkForm: FormGroup = this.fb.group({
    linkType: ['workflow', [Validators.required]],
    linkId: ['', [Validators.required]],
    relationship: ['related'],
    sourceUri: [''],
    sourceLabel: [''],
    confidence: [0.7],
  });

  constructor(
    private fb: FormBuilder,
    private pursuitsService: PursuitService,
    private automationsService: AutomationsService,
    private workflowService: WorkflowService,
    private notification: NzNotificationService,
    private router: Router,
    private route: ActivatedRoute
  ) {}

  ngOnInit(): void {
    this.routeSub = this.route.queryParamMap.subscribe((params) => {
      const selectedId = params.get('selected') || '';
      const evidenceUri = params.get('evidence') || '';
      const decisionId = params.get('decision') || '';
      if (selectedId === this.requestedPursuitId && evidenceUri === this.requestedEvidenceUri && decisionId === this.highlightedDecisionId) {
        return;
      }
      this.requestedPursuitId = selectedId;
      this.requestedEvidenceUri = evidenceUri;
      this.highlightedDecisionId = decisionId;
      if (selectedId && this.pursuits.length) {
        this.selectPursuitById(selectedId, false);
        return;
      }
      if (evidenceUri && this.selected?.pursuit.id === selectedId) {
        this.openRequestedEvidence();
      }
    });
    this.load();
  }

  ngOnDestroy(): void {
    this.routeSub?.unsubscribe();
  }

  load(): void {
    this.loading = true;
    this.pursuitsService.dashboard().subscribe({
      next: (dashboard) => {
        this.dashboard = dashboard;
        this.loadPursuits();
      },
      error: (error) => {
        this.loading = false;
        this.notification.error('Pursuits unavailable', error?.error?.error || 'Failed to load pursuit dashboard.');
      },
    });
  }

  loadPursuits(): void {
    this.pursuitsService.list(this.includeArchived).subscribe({
      next: (pursuits) => {
        this.pursuits = pursuits || [];
        this.loading = false;
        if (this.requestedPursuitId) {
          this.selectPursuitById(this.requestedPursuitId, false);
          return;
        }
        if (!this.selected && this.pursuits.length) {
          this.selectPursuit(this.pursuits[0]);
          return;
        }
        if (this.selected) {
          const refreshed = this.pursuits.find((item) => item.id === this.selected?.pursuit.id);
          if (refreshed) {
            this.selectPursuit(refreshed);
          }
        }
      },
      error: (error) => {
        this.loading = false;
        this.notification.error('Pursuits unavailable', error?.error?.error || 'Failed to load pursuits.');
      },
    });
  }

  selectPursuit(pursuit: IPursuit, updateRoute: boolean = true): void {
    if (this.selected?.pursuit.id === pursuit.id && !this.detailLoading) {
      if (updateRoute) {
        this.setSelectedQuery(pursuit.id);
      }
      return;
    }
    this.requestedPursuitId = pursuit.id;
    this.loadPursuitDetail(pursuit.id, updateRoute);
  }

  selectFirstFromQueue(queue: IPursuitListItem[] = []): void {
    if (!queue.length) {
      return;
    }
    this.selectPursuit(queue[0].pursuit);
  }

  openDashboardDecision(card: IPursuitDashboardDecision): void {
    this.selectPursuit(card.pursuit);
  }

  resolveDashboardDecision(card: IPursuitDashboardDecision, approved: boolean): void {
    if (this.resolvingDecisionId || !this.canResolveDecision(card.decision)) {
      return;
    }
    this.detailLoading = true;
    this.pursuitsService.get(card.pursuit.id).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.detailLoading = false;
        this.setSelectedQuery(detail.pursuit.id);
        const freshDecision = detail.decisionQueue.find((item) => item.id === card.decision.id) || card.decision;
        this.resolveDecision(freshDecision, approved);
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Decision unavailable', error?.error?.error || 'HAI could not load the pursuit behind this decision.');
      },
    });
  }

  private selectPursuitById(id: string, updateRoute: boolean): void {
    if (!id) {
      return;
    }
    if (this.selected?.pursuit.id === id && !this.detailLoading) {
      this.openRequestedEvidence();
      return;
    }
    const listed = this.pursuits.find((item) => item.id === id);
    if (listed) {
      this.selectPursuit(listed, updateRoute);
      return;
    }
    this.loadPursuitDetail(id, updateRoute);
  }

  private loadPursuitDetail(id: string, updateRoute: boolean): void {
    this.detailLoading = true;
    this.delegationPackage = undefined;
    this.showContextEditor = false;
    this.pursuitsService.get(id).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.detailLoading = false;
        if (updateRoute) {
          this.setSelectedQuery(detail.pursuit.id);
        }
        this.openRequestedEvidence();
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Pursuit unavailable', error?.error?.error || 'Failed to load pursuit detail.');
      },
    });
  }

  private setSelectedQuery(id?: string): void {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { selected: id || null, evidence: null, decision: null },
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
  }

  private openRequestedEvidence(): void {
    const uri = this.requestedEvidenceUri;
    if (!uri || !this.selected?.pursuit?.id || this.detailLoading) {
      return;
    }
    this.requestedEvidenceUri = '';
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { evidence: null },
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
    this.openSource(uri);
  }

  isHighlightedDecision(decision: IPursuitDecision): boolean {
    return !!this.highlightedDecisionId && decision.id === this.highlightedDecisionId;
  }

  highlightedDecision(): IPursuitDecision | undefined {
    if (!this.highlightedDecisionId || !this.selected?.decisionQueue?.length) {
      return undefined;
    }
    return this.selected.decisionQueue.find((decision) => decision.id === this.highlightedDecisionId);
  }

  createPursuit(): void {
    if (this.createForm.invalid) {
      this.createForm.markAllAsTouched();
      return;
    }
    this.creating = true;
    this.pursuitsService.create({
      ...this.createForm.value,
      sourceOfCreation: 'dashboard',
    }).subscribe({
      next: (pursuit) => {
        this.creating = false;
        this.showCreate = false;
        this.createForm.reset({ domain: 'operations' });
        this.notification.success('Pursuit created', 'HAI can now link workflows, memory, sources, and approvals to it.');
        this.load();
        this.selectPursuit(pursuit);
      },
      error: (error) => {
        this.creating = false;
        this.notification.error('Create failed', error?.error?.error || 'The pursuit could not be created.');
      },
    });
  }

  openContextEditor(): void {
    if (!this.selected) {
      return;
    }
    const pursuit = this.selected.pursuit;
    this.contextForm.reset({
      description: pursuit.description || '',
      whyItMatters: pursuit.whyItMatters || '',
      desiredOutcome: pursuit.desiredOutcome || '',
      currentStateSummary: pursuit.currentStateSummary || '',
      nextRecommendedAction: pursuit.nextRecommendedAction || '',
      completionDefinition: pursuit.completionDefinition || '',
    });
    this.showContextEditor = true;
  }

  savePursuitContext(): void {
    if (!this.selected || this.detailLoading) {
      return;
    }
    this.detailLoading = true;
    const pursuitID = this.selected.pursuit.id;
    this.pursuitsService.update(pursuitID, this.contextForm.value).subscribe({
      next: (pursuit) => {
        if (this.selected?.pursuit.id === pursuitID) {
          this.selected = { ...this.selected, pursuit };
        }
        this.detailLoading = false;
        this.showContextEditor = false;
        this.notification.success('Pursuit context saved', 'HAI will use the updated goal context for matching, planning, and safety checks.');
        this.load();
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Context update failed', error?.error?.error || 'HAI could not save the pursuit context.');
      },
    });
  }

  runIntake(): void {
    if (!this.selected || this.intakeForm.invalid) {
      this.intakeForm.markAllAsTouched();
      return;
    }
    this.intakeRunning = true;
    this.pursuitsService.intake(this.selected.pursuit.id, {
      ...this.intakeForm.value,
      projectKey: this.selected.pursuit.projectKey,
      trigger: 'pursuit_dashboard',
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.intakeRunning = false;
        this.notification.success('Workflow created', 'The signal was linked to this pursuit and sent through workflow intake.');
        this.load();
      },
      error: (error) => {
        this.intakeRunning = false;
        this.notification.error('Intake failed', error?.error?.error || 'HAI could not create a workflow from this signal.');
      },
    });
  }

  routeIntake(): void {
    if (this.routedIntakeForm.invalid) {
      this.routedIntakeForm.markAllAsTouched();
      return;
    }
    this.routedIntakeRunning = true;
    this.pursuitsService.routeIntake({
      ...this.routedIntakeForm.value,
      trigger: 'pursuit_dashboard_global_intake',
    }).subscribe({
      next: (result) => {
        this.routedIntakeRunning = false;
        this.routedIntakeForm.patchValue({ input: '', sourceLabel: '', sourceUri: '' });
        if (result.detail) {
          this.selected = result.detail;
        }
        this.notification.success(
          result.createdCandidate ? 'Pursuit candidate created' : 'Input routed',
          result.message || 'HAI matched the input and created governed workflow context.'
        );
        this.load();
        if (result.pursuitId) {
          this.selectPursuitById(result.pursuitId, true);
        }
      },
      error: (error) => {
        this.routedIntakeRunning = false;
        this.notification.error('Routing failed', error?.error?.error || 'HAI could not route this input into pursuits.');
      },
    });
  }

  refreshSelectedSummary(): void {
    if (!this.selected) {
      return;
    }
    this.detailLoading = true;
    this.pursuitsService.refreshSummary(this.selected.pursuit.id).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.detailLoading = false;
        this.notification.success('Summary refreshed', 'Pursuit status, blockers, and completion state were recalculated.');
        this.load();
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Summary failed', error?.error?.error || 'HAI could not refresh this pursuit.');
      },
    });
  }

  completeSelectedReview(): void {
    if (!this.selected || this.reviewing) {
      return;
    }
    this.reviewing = true;
    this.pursuitsService.review(this.selected.pursuit.id, {
      action: 'complete',
      note: 'Scheduled pursuit review completed from the dashboard.',
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.reviewing = false;
        this.notification.success('Review recorded', 'The pursuit review was audited and scheduled forward.');
        this.load();
      },
      error: (error) => {
        this.reviewing = false;
        this.notification.error('Review failed', error?.error?.error || 'HAI could not record this pursuit review.');
      },
    });
  }

  snoozeSelectedReview(days: number = 3): void {
    if (!this.selected || this.reviewing) {
      return;
    }
    this.reviewing = true;
    this.pursuitsService.review(this.selected.pursuit.id, {
      action: 'snooze',
      snoozeDays: days,
      note: `Scheduled pursuit review snoozed for ${days} days from the dashboard.`,
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.reviewing = false;
        this.notification.success('Review snoozed', `The pursuit will return to the review queue in ${days} days.`);
        this.load();
      },
      error: (error) => {
        this.reviewing = false;
        this.notification.error('Snooze failed', error?.error?.error || 'HAI could not snooze this pursuit review.');
      },
    });
  }

  createFirstWorkflowPlan(): void {
    if (!this.selected || this.planning) {
      return;
    }
    this.planning = true;
    this.pursuitsService.plan(this.selected.pursuit.id, {
      requiresReview: this.selected.pursuit.riskLevel === 'high',
      reviewReason: this.selected.pursuit.riskLevel === 'high'
        ? 'High-risk pursuit planning requires Robert approval before execution.'
        : 'First pursuit workflow plan created from pursuit dashboard.',
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.planning = false;
        this.notification.success('Workflow plan created', 'The first workflow was created, linked, and sent through HAI workflow policy.');
        this.load();
      },
      error: (error) => {
        this.planning = false;
        this.notification.error('Planning failed', error?.error?.error || 'HAI could not create the first workflow for this pursuit.');
      },
    });
  }

  prepareDelegationPackage(): void {
    if (!this.selected || this.delegationLoading) {
      return;
    }
    this.delegationLoading = true;
    this.pursuitsService.delegationPackage(this.selected.pursuit.id).subscribe({
      next: (delegationPackage) => {
        this.delegationPackage = delegationPackage;
        this.delegationLoading = false;
        const title = delegationPackage.ready ? 'VA brief ready' : 'VA brief blocked';
        this.notification.info(title, delegationPackage.reason);
      },
      error: (error) => {
        this.delegationLoading = false;
        this.notification.error('VA brief unavailable', error?.error?.error || 'HAI could not prepare the delegation package.');
      },
    });
  }

  archiveSelected(): void {
    if (!this.selected) {
      return;
    }
    if (this.isClosedPursuit(this.selected.pursuit)) {
      this.reopenSelected();
      return;
    }
    const archived = true;
    this.pursuitsService.archive(this.selected.pursuit.id, archived).subscribe({
      next: () => {
        this.notification.success('Pursuit archived', 'The pursuit registry was updated.');
        this.selected = undefined;
        this.requestedPursuitId = '';
        this.setSelectedQuery();
        this.load();
      },
      error: (error) => this.notification.error('Archive failed', error?.error?.error || 'The pursuit could not be updated.'),
    });
  }

  reopenSelected(): void {
    if (!this.selected || !this.isClosedPursuit(this.selected.pursuit)) {
      return;
    }
    const pursuit = this.selected.pursuit;
    this.pursuitsService.reopen(pursuit.id).subscribe({
      next: () => {
        this.notification.success('Pursuit reopened', 'HAI can now prepare new governed work for this pursuit.');
        this.loadPursuitDetail(pursuit.id, false);
        this.load();
      },
      error: (error) => this.notification.error('Reopen failed', error?.error?.error || 'HAI could not reopen this pursuit.'),
    });
  }

  isClosedPursuit(pursuit: IPursuit): boolean {
    return pursuit.archived || pursuit.status === 'completed' || pursuit.completionState === 'verified';
  }

  openWorkflow(workflowId?: string): void {
    this.router.navigate(['/workflow-engine'], { queryParams: workflowId ? { workflowId } : undefined });
  }

  verifySelectedEvidence(): void {
    if (!this.selected) {
      return;
    }
    const pursuit = this.selected.pursuit;
    this.router.navigate(['/grounded-answers'], {
      queryParams: {
        pursuitId: pursuit.id,
        projectKey: pursuit.projectKey || undefined,
        question: pursuit.nextRecommendedAction || pursuit.desiredOutcome || pursuit.title,
      },
    });
  }

  openAssistant(): void {
    if (!this.selected) {
      return;
    }
    const pursuit = this.selected.pursuit;
    this.router.navigate(['/task-blueprint'], {
      queryParams: {
        pursuitId: pursuit.id,
        projectKey: pursuit.projectKey || undefined,
        request: pursuit.nextRecommendedAction || pursuit.desiredOutcome || pursuit.title,
      },
    });
  }

  openAction(action: IPursuitAction): void {
    if (action.workflowId) {
      this.openWorkflow(action.workflowId);
      return;
    }
    this.inspectedAction = action;
  }

  closeAction(): void {
    this.inspectedAction = undefined;
  }

  openAutomations(): void {
    this.router.navigate(['/home']);
  }

  canStopAutomation(automation: IPursuitAutomation): boolean {
    return automation.launchType === 'agent_runtime' &&
      !!automation.id &&
      ['hermes', 'odysseus', 'openclaw'].includes((automation.runtimeType || '').toLowerCase());
  }

  stopRuntimeAutomation(automation: IPursuitAutomation): void {
    if (!this.canStopAutomation(automation) || this.stoppingAutomationId) {
      return;
    }
    this.stoppingAutomationId = automation.id;
    this.automationsService.stopRuntimeTask(automation.id).subscribe({
      next: (result) => {
        this.stoppingAutomationId = '';
        const title = result.status === 'stopping' ? 'Runtime stop requested' : 'Runtime stop response';
        const evidence = result.evidenceUri ? ` Evidence: ${result.evidenceUri}` : '';
        this.notification.success(title, `${result.message || `${result.runtimeId} returned ${result.status}.`}${evidence}`);
        this.reloadSelectedAfterDecision();
      },
      error: (error) => {
        this.stoppingAutomationId = '';
        this.notification.error(
          'Runtime stop blocked',
          error?.error?.error || error?.error?.message || 'HAI could not stop this runtime task.'
        );
      },
    });
  }

  runtimeEvidenceUri(attempt: IAutomationLaunchEvent): string {
    return attempt?.id ? `automation-launch://${attempt.id}` : '';
  }

  runtimeEvidenceLabel(attempt: IAutomationLaunchEvent): string {
    return this.runtimeEvidenceUri(attempt) || 'No persisted launch-event id';
  }

  runtimeEvidenceTitle(attempt: IAutomationLaunchEvent): string {
    const uri = this.runtimeEvidenceUri(attempt);
    if (!uri) {
      return 'This runtime attempt is missing a persisted launch-event ID, so it cannot be used as exact evidence.';
    }
    return `Exact runtime evidence URI: ${uri}`;
  }

  copyRuntimeEvidence(attempt: IAutomationLaunchEvent): void {
    this.copyEvidenceUri(this.runtimeEvidenceUri(attempt));
  }

  copyEvidenceUri(uri?: string): void {
    const value = (uri || '').trim();
    if (!value) {
      this.notification.warning('Evidence missing', 'This record does not have a stable evidence URI.');
      return;
    }
    if (!navigator?.clipboard?.writeText) {
      this.notification.info('Evidence URI', value);
      return;
    }
    navigator.clipboard.writeText(value).then(
      () => this.notification.success('Evidence URI copied', value),
      () => this.notification.info('Evidence URI', value)
    );
  }

  inspectRuntimeEvidence(attempt: IAutomationLaunchEvent): void {
    if (!attempt?.id) {
      this.notification.warning('Runtime evidence missing', 'This runtime attempt is missing a persisted launch-event ID.');
      return;
    }
    this.inspectedRuntimeEvidence = attempt;
  }

  closeRuntimeEvidence(): void {
    this.inspectedRuntimeEvidence = undefined;
  }

  inspectEvidence(record: IPursuitEvidenceResolution): void {
    this.inspectedEvidence = record;
  }

  closeEvidence(): void {
    this.inspectedEvidence = undefined;
  }

  runtimeEvidenceType(attempt?: IAutomationLaunchEvent): string {
    if (!attempt) {
      return 'runtime';
    }
    if ((attempt.launchType || '').toLowerCase() === 'agent_runtime_stop') {
      return 'runtime stop';
    }
    return attempt.launchType || 'runtime attempt';
  }

  runtimeEvidenceDuration(attempt?: IAutomationLaunchEvent): string {
    if (!attempt) {
      return '-';
    }
    return `${attempt.durationMs || 0} ms`;
  }

  runtimeTraceHeadline(trace?: IAutomationRuntimeRouteTrace): string {
    if (!trace) {
      return 'No routing trace recorded';
    }
    return [
      trace.intent || trace.runtimeId || 'runtime task',
      trace.riskLevel ? `${trace.riskLevel} risk` : '',
      trace.executionMode || '',
    ].filter(Boolean).join(' / ');
  }

  runtimeTraceSummary(trace?: IAutomationRuntimeRouteTrace): string {
    if (!trace) {
      return 'This runtime attempt did not return a route trace.';
    }
    const parts = [
      this.runtimeTraceCount(trace.recommendedSkills, 'skill'),
      this.runtimeTraceCount(trace.visibleProviders, 'provider'),
      this.runtimeTraceCount(trace.visibleTools, 'tool'),
      this.runtimeTraceCount(trace.relevantMaps, 'map'),
      this.runtimeTraceCount(trace.blockedSurfaces, 'blocked surface'),
      this.runtimeTraceCount(trace.requiredControls, 'control'),
      this.runtimeTraceCount(trace.validationChecklist, 'validation check'),
    ].filter(Boolean);
    return parts.length ? parts.join(' / ') : 'Route trace recorded without expanded planning details.';
  }

  runtimeTraceItems(values?: string[], limit: number = 4): string[] {
    return (values || []).filter(Boolean).slice(0, limit);
  }

  runtimeTraceMore(values?: string[], limit: number = 4): number {
    return Math.max(0, (values || []).filter(Boolean).length - limit);
  }

  runtimeTraceTitle(label: string, values?: string[]): string {
    const items = (values || []).filter(Boolean);
    return items.length ? `${label}: ${items.join(', ')}` : `${label}: none recorded`;
  }

  private runtimeTraceCount(values: string[] | undefined, label: string): string {
    const count = (values || []).filter(Boolean).length;
    if (!count) {
      return '';
    }
    return `${count} ${label}${count === 1 ? '' : 's'}`;
  }

  addLink(): void {
    if (!this.selected || this.linkForm.invalid) {
      this.linkForm.markAllAsTouched();
      return;
    }
    this.detailLoading = true;
    this.pursuitsService.link(this.selected.pursuit.id, {
      ...this.linkForm.value,
    }).subscribe({
      next: () => {
        this.notification.success('Link added', 'The pursuit now includes this operational record.');
        this.linkForm.patchValue({
          linkId: '',
          sourceUri: '',
          sourceLabel: '',
          confidence: 0.7,
        });
        this.reloadSelectedAfterDecision();
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Link blocked', error?.error?.error || 'The record could not be linked to this pursuit.');
      },
    });
  }

  deleteLink(linkId: string): void {
    if (!this.selected || !linkId) {
      return;
    }
    this.detailLoading = true;
    this.pursuitsService.deleteLink(this.selected.pursuit.id, linkId).subscribe({
      next: () => {
        this.notification.success('Link removed', 'Incorrect pursuit link detached.');
        this.reloadSelectedAfterDecision();
      },
      error: (error) => {
        this.detailLoading = false;
        this.notification.error('Detach blocked', error?.error?.error || 'The pursuit link could not be removed.');
      },
    });
  }

  openSource(uri?: string): void {
    const value = (uri || '').trim();
    if (!value) {
      return;
    }
    const runtimeAttempt = this.runtimeAttemptFromEvidenceUri(value);
    if (runtimeAttempt) {
      this.inspectRuntimeEvidence(runtimeAttempt);
      return;
    }
    if (!this.isBrowserNavigableUri(value) && this.selected?.pursuit?.id) {
      this.resolveEvidence(value);
      return;
    }
    window.open(value, '_blank', 'noopener');
  }

  private resolveEvidence(uri: string): void {
    if (!this.selected?.pursuit?.id || this.resolvingEvidenceUri) {
      return;
    }
    this.resolvingEvidenceUri = uri;
    this.pursuitsService.resolveEvidence(this.selected.pursuit.id, uri).subscribe({
      next: (record) => {
        this.resolvingEvidenceUri = '';
        if (record.runtimeAttempt) {
          this.inspectRuntimeEvidence(record.runtimeAttempt);
          return;
        }
        this.inspectEvidence(record);
      },
      error: (error) => {
        this.resolvingEvidenceUri = '';
        this.notification.warning(
          'Evidence unavailable',
          error?.error?.error || `${uri} is not linked to this pursuit.`
        );
      },
    });
  }

  private isBrowserNavigableUri(uri: string): boolean {
    return /^(https?:|mailto:|tel:|file:)/i.test(uri);
  }

  private runtimeAttemptFromEvidenceUri(uri: string): IAutomationLaunchEvent | undefined {
    const prefix = 'automation-launch://';
    if (!uri.startsWith(prefix)) {
      return undefined;
    }
    const id = uri.slice(prefix.length).trim().toLowerCase();
    if (!id || !this.selected?.runtimeAttempts?.length) {
      return undefined;
    }
    return this.selected.runtimeAttempts.find((attempt) => (attempt.id || '').toLowerCase() === id);
  }

  statusColor(status: string): string {
    switch (status) {
      case 'completed':
        return 'success';
      case 'blocked':
        return 'error';
      case 'waiting':
        return 'warning';
      case 'archived':
        return 'default';
      default:
        return 'processing';
    }
  }

  riskColor(risk: string): string {
    switch (risk) {
      case 'high':
        return 'error';
      case 'medium':
        return 'warning';
      case 'low':
        return 'success';
      default:
        return 'default';
    }
  }

  count(key: string): number {
    return Number(this.dashboard?.counts?.[key] || 0);
  }

  topQueue(): IPursuitListItem[] {
    return [
      ...(this.dashboard?.needsRobert || []),
      ...(this.dashboard?.reviewDue || []),
      ...(this.dashboard?.planningNeeded || []),
      ...(this.dashboard?.blocked || []),
      ...(this.dashboard?.stale || []),
    ].slice(0, 6);
  }

  queueItemLabel(item: IPursuitListItem): string {
    if (item.blocked) {
      return 'blocked';
    }
    if (item.needsRobert) {
      return 'needs Robert';
    }
    if (item.reviewDue) {
      return 'review due';
    }
    if (item.planningNeeded) {
      return 'needs planning';
    }
    if (item.stale) {
      return 'stale';
    }
    return 'attention';
  }

  queueItemColor(item: IPursuitListItem): string {
    if (item.blocked) {
      return 'error';
    }
    if (item.needsRobert || item.reviewDue || item.planningNeeded) {
      return 'warning';
    }
    return 'processing';
  }

  actionOwnerLabel(action: IPursuitAction): string {
    return action.requiresApproval ? `${action.owner} approval` : action.owner;
  }

  blockerOwnerLabel(blocker: IPursuitBlocker): string {
    return blocker.followUpAt ? `${blocker.owner} · follow up ${new Date(blocker.followUpAt).toLocaleDateString()}` : blocker.owner;
  }

  canResolveDecision(decision: IPursuitDecision): boolean {
    return decision.status === 'pending' &&
      (
        decision.decisionType === 'approval' ||
        decision.decisionType === 'proposal' ||
        decision.decisionType === 'pursuit_next_action' ||
        decision.decisionType === 'pursuit_candidate_review' ||
        decision.decisionType === 'runtime_attempt_review' ||
        decision.decisionType === 'pursuit_completion_review'
      );
  }

  resolveDecision(decision: IPursuitDecision, approved: boolean): void {
    if (!this.selected || this.resolvingDecisionId || !this.canResolveDecision(decision)) {
      return;
    }
    if (decision.decisionType === 'approval' && decision.workflowId) {
      this.resolveWorkflowApproval(decision, approved);
      return;
    }
    if (decision.decisionType === 'proposal' && decision.workflowId) {
      this.resolveWorkflowProposal(decision, approved);
      return;
    }
    if (decision.decisionType === 'pursuit_next_action') {
      this.resolvePursuitNextAction(decision, approved);
      return;
    }
    if (decision.decisionType === 'runtime_attempt_review') {
      this.resolveRuntimeAttemptReview(decision, approved);
      return;
    }
    if (decision.decisionType === 'pursuit_completion_review') {
      this.resolvePursuitCompletionReview(decision, approved);
      return;
    }
    if (decision.decisionType === 'pursuit_candidate_review') {
      this.resolvePursuitCandidateReview(decision, approved);
      return;
    }
    this.notification.info('Open workflow', 'Open the linked workflow to inspect this decision record.');
  }

  private resolveWorkflowApproval(decision: IPursuitDecision, approved: boolean): void {
    if (!decision.workflowId) {
      return;
    }
    this.resolvingDecisionId = decision.id;
    this.workflowService.resolveApproval(decision.workflowId, {
      approved,
      note: approved ? decision.yesConsequence : decision.noConsequence,
      actor: 'Robert',
    }).subscribe({
      next: () => {
        this.resolvingDecisionId = '';
        this.notification.success('Approval recorded', approved ? 'Workflow approved through the audited gate.' : 'Workflow rejected and blocked for review.');
        this.reloadSelectedAfterDecision();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error('Approval blocked', error?.error?.error || 'The workflow approval could not be recorded.');
      },
    });
  }

  private resolveWorkflowProposal(decision: IPursuitDecision, approved: boolean): void {
    if (!decision.workflowId) {
      return;
    }
    const proposalId = this.proposalIdFromDecision(decision);
    if (!proposalId) {
      this.notification.error('Proposal unavailable', 'The proposal ID is missing from this decision card.');
      return;
    }
    this.resolvingDecisionId = decision.id;
    this.workflowService.resolveProposal(decision.workflowId, proposalId, {
      approved,
      status: approved ? 'approved' : 'rejected',
      selectedOption: approved ? decision.yesLabel : decision.noLabel,
      note: approved ? decision.yesConsequence : decision.noConsequence,
      actor: 'Robert',
    }).subscribe({
      next: () => {
        this.resolvingDecisionId = '';
        this.notification.success('Proposal recorded', approved ? 'Proposal accepted through the workflow audit trail.' : 'Proposal rejected for revision.');
        this.reloadSelectedAfterDecision();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error('Proposal blocked', error?.error?.error || 'The proposal could not be resolved.');
      },
    });
  }

  private resolvePursuitNextAction(decision: IPursuitDecision, approved: boolean): void {
    if (!this.selected) {
      return;
    }
    this.resolvingDecisionId = decision.id;
    if (approved) {
      this.pursuitsService.intake(this.selected.pursuit.id, {
        input: decision.recommended,
        projectKey: this.selected.pursuit.projectKey,
        sourceType: 'pursuit_decision',
        sourceId: decision.id,
        sourceUri: decision.evidenceUri,
        sourceLabel: decision.evidenceLabel || 'Robert approved pursuit next action',
        contentType: decision.decisionType,
        trigger: 'pursuit_decision_approved',
        requiresReview: decision.requiresApproval,
        reviewReason: decision.reason,
      }).subscribe({
        next: (detail) => {
          this.selected = detail;
          this.resolvingDecisionId = '';
          this.notification.success('Workflow created', 'The pursuit decision became a governed workflow item.');
          this.load();
        },
        error: (error) => {
          this.resolvingDecisionId = '';
          this.notification.error('Workflow creation blocked', error?.error?.error || 'HAI could not create the governed workflow.');
        },
      });
      return;
    }
    this.pursuitsService.resolveDecision(this.selected.pursuit.id, {
      decisionId: decision.id,
      decisionType: decision.decisionType,
      approved: false,
      reason: decision.reason,
      note: decision.noConsequence || `Robert rejected the proposed next action: ${decision.recommended}`,
      evidenceUri: decision.evidenceUri,
      evidenceLabel: decision.evidenceLabel,
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.resolvingDecisionId = '';
        this.notification.success('Decision recorded', 'The pursuit decision is now resolved in the audit trail.');
        this.load();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error('Decision blocked', error?.error?.error || 'The pursuit decision could not be recorded.');
      },
    });
  }

  private resolveRuntimeAttemptReview(decision: IPursuitDecision, approved: boolean): void {
    if (!this.selected) {
      return;
    }
    this.resolvingDecisionId = decision.id;
    this.pursuitsService.resolveDecision(this.selected.pursuit.id, {
      decisionId: decision.id,
      decisionType: decision.decisionType,
      approved,
      reason: decision.reason,
      note: approved ? decision.yesConsequence || decision.recommended : decision.noConsequence || 'Keep runtime attempt blocked until reviewed.',
      evidenceUri: decision.evidenceUri,
      evidenceLabel: decision.evidenceLabel,
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.resolvingDecisionId = '';
        this.notification.success(
          approved ? 'Recovery workflow created' : 'Runtime attempt kept blocked',
          approved
            ? 'HAI created a governed recovery workflow through the audited decision path.'
            : 'The runtime attempt remains blocked and is removed from the Robert-only decision queue.'
        );
        this.load();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error(
          approved ? 'Recovery workflow blocked' : 'Decision blocked',
          error?.error?.error || 'The runtime recovery decision could not be recorded.'
        );
      },
    });
  }

  private resolvePursuitCompletionReview(decision: IPursuitDecision, approved: boolean): void {
    if (!this.selected) {
      return;
    }
    this.resolvingDecisionId = decision.id;
    this.pursuitsService.resolveDecision(this.selected.pursuit.id, {
      decisionId: decision.id,
      decisionType: decision.decisionType,
      approved,
      reason: decision.reason,
      note: approved
        ? decision.yesConsequence || 'Robert approved verified pursuit completion.'
        : decision.noConsequence || 'Robert kept the pursuit active after completion review.',
      evidenceUri: decision.evidenceUri,
      evidenceLabel: decision.evidenceLabel,
    }).subscribe({
      next: (detail) => {
        this.selected = detail;
        this.resolvingDecisionId = '';
        this.notification.success(
          approved ? 'Pursuit completed' : 'Pursuit kept active',
          approved
            ? 'Verified completion and the Robert decision were recorded in the audit trail.'
            : 'The completion review decision was recorded.'
        );
        this.load();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error(
          approved ? 'Completion blocked' : 'Decision blocked',
          error?.error?.error || 'The completion review decision could not be recorded.'
        );
      },
    });
  }

  private resolvePursuitCandidateReview(decision: IPursuitDecision, approved: boolean): void {
    if (!this.selected) {
      return;
    }
    this.resolvingDecisionId = decision.id;
    if (approved) {
      this.pursuitsService.plan(this.selected.pursuit.id, {
        requiresReview: decision.riskLevel === 'high',
        reviewReason: decision.reason,
      }).subscribe({
        next: (detail) => {
          this.selected = detail;
          this.resolvingDecisionId = '';
          this.notification.success('Candidate accepted', 'HAI converted the candidate into governed pursuit work.');
          this.load();
        },
        error: (error) => {
          this.resolvingDecisionId = '';
          this.notification.error('Candidate blocked', error?.error?.error || 'HAI could not accept and plan this candidate.');
        },
      });
      return;
    }
    this.pursuitsService.archive(this.selected.pursuit.id, true).subscribe({
      next: () => {
        this.selected = undefined;
        this.resolvingDecisionId = '';
        this.notification.success('Candidate archived', 'The auto-created candidate was removed from active queues.');
        this.load();
      },
      error: (error) => {
        this.resolvingDecisionId = '';
        this.notification.error('Archive blocked', error?.error?.error || 'HAI could not archive this candidate.');
      },
    });
  }

  private proposalIdFromDecision(decision: IPursuitDecision): string {
    return decision.id.startsWith('proposal:') ? decision.id.substring('proposal:'.length) : '';
  }

  private reloadSelectedAfterDecision(): void {
    if (!this.selected) {
      this.load();
      return;
    }
    this.loadPursuitDetail(this.selected.pursuit.id, false);
    this.load();
  }

  trackPursuit(_: number, pursuit: IPursuit): string {
    return pursuit.id;
  }
}
