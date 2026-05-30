import { InjectionToken } from '@angular/core';
import { ILLMPolicyService } from '../llm-policy.service.interface';

export const LLM_POLICY_SERVICE_TOKEN =
  new InjectionToken<ILLMPolicyService>('LLM_POLICY_SERVICE_TOKEN');
