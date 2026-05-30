import { InjectionToken } from '@angular/core';
import { IVerificationService } from '../verification.service.interface';

export const VERIFICATION_SERVICE_TOKEN =
  new InjectionToken<IVerificationService>('VERIFICATION_SERVICE_TOKEN');
