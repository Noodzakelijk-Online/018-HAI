import { InjectionToken } from '@angular/core';
import { ISystemStatusService } from './system-status.service.interface';

export const SYSTEM_STATUS_SERVICE_TOKEN =
  new InjectionToken<ISystemStatusService>('SYSTEM_STATUS_SERVICE_TOKEN');
