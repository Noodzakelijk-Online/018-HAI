import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  IWorkflowDashboard,
  IWorkflowItem,
  IWorkflowOverview,
  IWorkflowRecord,
  IWorkflowRunSummary,
} from '../../models/workflow.model.interface';
import { WORKFLOW_SERVICE_TOKEN } from '../../services/workflow/workflow.service.token';
import { IWorkflowService } from '../../services/workflow.service.interface';

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
  includeArchived = false;
  loading = false;
  saving = false;

  intakeForm: FormGroup = this.fb.group({
    input: [
      'Email from lawyer about Vivare legal hearing tomorrow. Draft formal Dutch reply and attach evidence.',
      [Validators.required],
    ],
    projectKey: ['Vivare dispute'],
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
    message: ['Robert approved controlled workflow execution.'],
    approved: [false],
  });

  approvalForm: FormGroup = this.fb.group({
    note: ['Robert approved controlled workflow execution.'],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(WORKFLOW_SERVICE_TOKEN) private workflowService: IWorkflowService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.loading = true;
    this.workflowService.overview().subscribe({
      next: (overview) => (this.overview = overview),
      error: () => this.notification.error('Error', 'Failed to load workflow overview.'),
    });
    this.workflowService.dashboard().subscribe({
      next: (dashboard) => (this.dashboard = dashboard),
      error: () => this.notification.error('Error', 'Failed to load workflow dashboard.'),
    });
    this.workflowService.items(this.includeArchived).subscribe({
      next: (items) => {
        this.items = items;
        this.loading = false;
      },
      error: () => {
        this.items = [];
        this.loading = false;
        this.notification.error('Error', 'Failed to load workflow inbox.');
      },
    });
    this.workflowService.approvals().subscribe({
      next: (items) => (this.approvalItems = items),
      error: () => this.notification.error('Error', 'Failed to load approval queue.'),
    });
  }

  intake(): void {
    if (this.intakeForm.invalid) {
      return;
    }
    this.saving = true;
    this.workflowService.intake(this.intakeForm.value).subscribe({
      next: (record) => {
        this.selected = record;
        this.saving = false;
        this.notification.success('Workflow created', 'Input classified, checklist generated, and audit event recorded.');
        this.refresh();
      },
      error: () => {
        this.saving = false;
        this.notification.error('Error', 'Failed to intake workflow input.');
      },
    });
  }

  open(item: IWorkflowItem): void {
    this.workflowService.get(item.id).subscribe({
      next: (record) => (this.selected = record),
      error: () => this.notification.error('Error', 'Failed to open workflow.'),
    });
  }

  transition(): void {
    if (!this.selected || this.transitionForm.invalid) {
      return;
    }
    this.workflowService.transition(this.selected.item.id, this.transitionForm.value).subscribe({
      next: (record) => {
        this.selected = record;
        this.notification.success('State updated', 'Workflow transition was validated and audited.');
        this.refresh();
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
        this.selected = record;
        this.notification.success('Approval updated', approved ? 'Workflow approved for execution.' : 'Workflow rejected and blocked.');
        this.refresh();
      },
      error: () => this.notification.error('Error', 'Failed to update workflow approval.'),
    });
  }

  runDue(): void {
    this.workflowService.runDue({ limit: 10 }).subscribe({
      next: (summary) => {
        this.runSummary = summary;
        this.notification.success('Worker run complete', `${summary.completed} completed, ${summary.retried} retried, ${summary.blocked} blocked.`);
        this.refresh();
      },
      error: () => this.notification.error('Error', 'Workflow worker run failed.'),
    });
  }

  markChecklist(itemId: string, status: string): void {
    if (!this.selected) {
      return;
    }
    this.workflowService.updateChecklistItem(this.selected.item.id, itemId, { status }).subscribe({
      next: (record) => (this.selected = record),
      error: () => this.notification.error('Error', 'Failed to update checklist item.'),
    });
  }

  optionLines(options?: string): string[] {
    return (options || '').split('\n').filter((option) => !!option.trim());
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }
}
