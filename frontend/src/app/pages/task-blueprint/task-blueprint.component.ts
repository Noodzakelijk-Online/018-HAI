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

  planForm: FormGroup = this.fb.group({
    request: [
      'Implement a completion-first context and routing workflow for 018-HAI.',
      [Validators.required],
    ],
    projectKey: ['018-HAI'],
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
        successCriteria: this.criteria(),
      })
      .subscribe({
        next: (plan) => {
          this.plan = plan;
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
        successCriteria: this.criteria(),
        executeAllowed: true,
      })
      .subscribe({
        next: (plan) => {
          this.plan = plan;
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
      next: (logs) => (this.logs = logs),
      error: () => (this.logs = []),
    });
  }

  loadReviewQueue(): void {
    this.taskPlanService.reviewQueue().subscribe({
      next: (items) => (this.reviewQueue = items),
      error: () => (this.reviewQueue = []),
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
}
