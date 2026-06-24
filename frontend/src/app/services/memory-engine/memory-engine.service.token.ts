import { InjectionToken } from '@angular/core';
import { IMemoryEngineService } from '../memory-engine.service.interface';

export const MEMORY_ENGINE_SERVICE_TOKEN = new InjectionToken<IMemoryEngineService>(
  'MEMORY_ENGINE_SERVICE_TOKEN'
);
