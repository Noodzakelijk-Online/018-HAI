import { InjectionToken } from '@angular/core';
import { IConnectedSourceService } from '../connected-source.service.interface';

export const CONNECTED_SOURCE_SERVICE_TOKEN =
  new InjectionToken<IConnectedSourceService>('CONNECTED_SOURCE_SERVICE_TOKEN');
