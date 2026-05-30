import { InjectionToken } from '@angular/core';
import { ITaskPlanService } from '../task-plan.service.interface';

export const TASK_PLAN_SERVICE_TOKEN =
  new InjectionToken<ITaskPlanService>('TASK_PLAN_SERVICE_TOKEN');
