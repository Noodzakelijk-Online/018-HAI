import { InjectionToken } from '@angular/core';
import { IWorkflowService } from '../workflow.service.interface';

export const WORKFLOW_SERVICE_TOKEN =
  new InjectionToken<IWorkflowService>('WORKFLOW_SERVICE_TOKEN');
