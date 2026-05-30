import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  ILLMPolicy,
  ILLMProvider,
  ILLMRouteDecision,
} from '../../models/llm-policy.model.interface';
import { LLM_POLICY_SERVICE_TOKEN } from '../../services/llm-policy/llm-policy.service.token';
import { ILLMPolicyService } from '../../services/llm-policy.service.interface';

@Component({
  selector: 'app-llm-policy',
  templateUrl: './llm-policy.component.html',
  styleUrls: ['./llm-policy.component.scss'],
})
export class LLMPolicyComponent implements OnInit {
  policy?: ILLMPolicy;
  logs: ILLMRouteDecision[] = [];
  decision?: ILLMRouteDecision;
  loading = false;
  routing = false;
  routeForm: FormGroup = this.fb.group({
    task: [
      'Fix a Go API bug and explain why the previous model failed validation.',
      [Validators.required],
    ],
    validationPassed: [true],
    previousModelId: [''],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(LLM_POLICY_SERVICE_TOKEN)
    private llmPolicyService: ILLMPolicyService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.loading = true;
    this.llmPolicyService.getPolicy().subscribe({
      next: (policy) => {
        this.policy = policy;
        this.loading = false;
      },
      error: () => {
        this.loading = false;
        this.notification.error('Error', 'Failed to load LLM routing policy.');
      },
    });
    this.loadLogs();
  }

  routeTask(): void {
    if (this.routeForm.invalid) {
      Object.values(this.routeForm.controls).forEach((control) => {
        control.markAsDirty();
        control.updateValueAndValidity();
      });
      return;
    }

    this.routing = true;
    const previousModelId = this.routeForm.value.previousModelId || undefined;
    this.llmPolicyService
      .routeTask({
        task: this.routeForm.value.task,
        validationPassed: this.routeForm.value.validationPassed,
        previousModelId,
      })
      .subscribe({
        next: (decision) => {
          this.routing = false;
          this.decision = decision;
          this.loadLogs();
        },
        error: () => {
          this.routing = false;
          this.notification.error('Error', 'Failed to route this task.');
        },
      });
  }

  loadLogs(): void {
    this.llmPolicyService.getLogs().subscribe({
      next: (logs) => (this.logs = logs),
      error: () => (this.logs = []),
    });
  }

  providerStatus(provider: ILLMProvider): string {
    if (!provider.enabled) {
      return 'disabled';
    }
    if (provider.paid) {
      return 'paid';
    }
    if (provider.local) {
      return 'local';
    }
    return 'free quota';
  }

  providerColor(provider: ILLMProvider): string {
    if (!provider.enabled) {
      return 'default';
    }
    if (provider.paid) {
      return 'red';
    }
    if (provider.local) {
      return 'green';
    }
    return 'blue';
  }

  tierColor(tier?: string): string {
    switch (tier) {
      case 'free':
        return 'green';
      case 'cheap':
        return 'cyan';
      case 'acceptable':
        return 'blue';
      case 'high':
        return 'gold';
      case 'expensive':
        return 'red';
      default:
        return 'default';
    }
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }
}
