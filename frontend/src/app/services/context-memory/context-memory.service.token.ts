import { InjectionToken } from '@angular/core';
import { IContextMemoryService } from '../context-memory.service.interface';

export const CONTEXT_MEMORY_SERVICE_TOKEN =
  new InjectionToken<IContextMemoryService>('CONTEXT_MEMORY_SERVICE_TOKEN');
