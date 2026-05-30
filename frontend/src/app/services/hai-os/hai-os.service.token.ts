import { InjectionToken } from '@angular/core';
import { IHAIOSService } from '../hai-os.service.interface';

export const HAI_OS_SERVICE_TOKEN =
  new InjectionToken<IHAIOSService>('HAI_OS_SERVICE_TOKEN');
