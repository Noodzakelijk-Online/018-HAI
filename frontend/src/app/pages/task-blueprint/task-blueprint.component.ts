import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  ICompletionPlan,
  IReviewQueueItem,
} from '../../models/task-plan.model.interface';
import { TASK_PLAN_SERVICE_TOKEN } from '../../services/task-plan/task-plan.service.token';
import { ITaskPlanService } from '../../services/task-plan.service.interface';

@Component({
  selector: 'app-task-blueprint',
  templateUrl: './task-blueprint.component.html',
  styleUrls: ['./task-blueprint.component.scss'],
})
export class TaskBlueprintComponent implements OnInit {
  plan?: ICompletionPlan;
  logs: ICompletionPlan[] = [];
  reviewQueue: IReviewQueueItem[] = [];
  loading = false;
  running = false;
  resolvingReviewId = '';

  planForm: FormGroup = this.fb.group({
    request: [
      'Implement a completion-first context and routing workflow for 018-HAI.',
      [Validators.required],
    ],
    projectKey: ['018-HAI'],
    automationId: [''],
    successCriteria: [''],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(TASK_PLAN_SERVICE_TOKEN)
    private taskPlanService: ITaskPlanService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.loadLogs();
    this.loadReviewQueue();
  }

  createPlan(): void {
    if (this.planForm.invalid) {
      Object.values(this.planForm.controls).forEach((control) => {
        control.markAsDirty();
        control.updateValueAndValidity();
      });
      return;
    }
    this.loading = true;
    this.taskPlanService
      .plan({
        request: this.planForm.value.request,
        projectKey: this.planForm.value.projectKey,
        automationId: this.planForm.value.automationId,
        successCriteria: this.criteria(),
      })
      .subscribe({
        next: (plan) => {
          this.plan = this.normalizePlan(plan);
          this.loading = false;
          this.loadLogs();
        },
        error: () => {
          this.loading = false;
          this.notification.error('Error', 'Failed to create task plan.');
        },
      });
  }

  runSuccessEngine(): void {
    if (this.planForm.invalid) {
      Object.values(this.planForm.controls).forEach((control) => {
        control.markAsDirty();
        control.updateValueAndValidity();
      });
      return;
    }
    this.running = true;
    this.taskPlanService
      .run({
        request: this.planForm.value.request,
        projectKey: this.planForm.value.projectKey,
        automationId: this.planForm.value.automationId,
        successCriteria: this.criteria(),
        executeAllowed: true,
      })
      .subscribe({
        next: (plan) => {
          this.plan = this.normalizePlan(plan);
          this.running = false;
          this.loadLogs();
          this.loadReviewQueue();
        },
        error: () => {
          this.running = false;
          this.notification.error('Error', 'Failed to run task success engine.');
        },
      });
  }

  loadLogs(): void {
    this.taskPlanService.logs().subscribe({
      next: (logs) => (this.logs = (logs || []).map((plan) => this.normalizePlan(plan))),
      error: () => (this.logs = []),
    });
  }

  loadReviewQueue(): void {
    this.taskPlanService.reviewQueue().subscribe({
      next: (items) => (this.reviewQueue = items || []),
      error: () => (this.reviewQueue = []),
    });
  }

  resolveReviewItem(item: IReviewQueueItem, approved: boolean): void {
    this.resolvingReviewId = item.id;
    this.taskPlanService
      .resolveReviewItem(item.id, {
        approved,
        note: approved
          ? 'Approved from Task Blueprint review queue.'
          : 'Rejected from Task Blueprint review queue.',
      })
      .subscribe({
        next: (result) => {
          this.resolvingReviewId = '';
          if (result.plan) {
            this.plan = this.normalizePlan(result.plan);
          }
          this.notification.success(
            approved ? 'Review approved' : 'Review rejected',
            result.plan ? 'The approved task was re-run through the success engine.' : 'The task remains blocked.'
          );
          this.loadLogs();
          this.loadReviewQueue();
        },
        error: () => {
          this.resolvingReviewId = '';
          this.notification.error('Error', 'Failed to resolve review item.');
        },
      });
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }

  private criteria(): string[] {
    return String(this.planForm.value.successCriteria || '')
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);
  }

  private normalizePlan(plan: ICompletionPlan): ICompletionPlan {
    plan.modelDecision.skipped = plan.modelDecision.skipped || [];
    plan.toolDecision.selectedTools = plan.toolDecision.selectedTools || [];
    plan.toolDecision.skippedTools = plan.toolDecision.skippedTools || [];
    plan.toolDecision.blockedTools = plan.toolDecision.blockedTools || [];
    plan.minimalityDecision.ladder = plan.minimalityDecision.ladder || [];
    plan.contextPlan.usedContext = plan.contextPlan.usedContext || [];
    plan.contextPlan.sourceContext = plan.contextPlan.sourceContext || [];
    plan.intake.successCriteria = plan.intake.successCriteria || [];
    plan.steps = plan.steps || [];
    plan.riskAssessment.reasons = plan.riskAssessment.reasons || [];
    plan.validationPlan.steps = plan.validationPlan.steps || [];
    plan.validationResult.checked = plan.validationResult.checked || [];
    plan.validationResult.failures = plan.validationResult.failures || [];
    plan.executionPlan.approvalRequiredFor = plan.executionPlan.approvalRequiredFor || [];
    plan.executionPlan.auditEvents = plan.executionPlan.auditEvents || [];
    plan.retryPolicy.escalationPath = plan.retryPolicy.escalationPath || [];
    plan.retryPolicy.escalateWhen = plan.retryPolicy.escalateWhen || [];
    plan.memoryUpdateProposals = plan.memoryUpdateProposals || [];
    plan.lessonsLearned = plan.lessonsLearned || [];
    plan.storedMemoryIds = plan.storedMemoryIds || [];
    plan.events = plan.events || [];
    if (plan.executionResult) {
      plan.executionResult.actions = plan.executionResult.actions || [];
      plan.executionResult.claims = plan.executionResult.claims || [];
    }
    return plan;
  }
}
