export interface ILLMPolicy {
  dailyPaidBudgetEur: number;
  paidCallsAllowed: boolean;
  localModelsAllowed: boolean;
  freeCloudQuotaAllowed: boolean;
  localFirst: boolean;
  cacheRepeatedPrompts: boolean;
  routeSimpleTasksToSmallModels: boolean;
  routeComplexTasksToBestAvailableFreeModel: boolean;
  requireApprovalBeforePaidUsage: boolean;
  tierOrder: string[];
  dailyBudgetUsedEur: number;
  inputTokensUsed: number;
  outputTokensUsed: number;
  providers: ILLMProvider[];
  inferenceInfrastructure: ILLMInferenceInfrastructure;
}

export interface ILLMInferenceInfrastructure {
  kvCacheLoadStrategy: string;
  disaggregatedServingVerified: boolean;
  dualPathInfrastructureAvailable: boolean;
  reason: string;
}

export interface ILLMProvider {
  id: string;
  name: string;
  enabled: boolean;
  local: boolean;
  paid: boolean;
  endpointUrl?: string;
  apiKeyEnv?: string;
  configured: boolean;
  readinessStatus?: string;
  readinessReason?: string;
  quotaRemaining: number;
  dailyBudgetEur: number;
  budgetUsedEur: number;
  inputTokensUsed: number;
  outputTokensUsed: number;
  models: ILLMModel[];
}

export interface ILLMProviderProbe {
  providerId: string;
  providerName: string;
  status: string;
  reason: string;
  endpointUrl?: string;
  httpStatus?: number;
  modelsSeen: number;
  durationMs: number;
  live: boolean;
  requiresReview: boolean;
  checkedAt: string;
  lastSuccessfulAt?: string;
}

export interface ILLMModel {
  id: string;
  name: string;
  tier: string;
  capabilities: string[];
  maxDifficulty: number;
  maxReasoning: string;
  estimatedCostEur: number;
  inputCostPerMillionTokensEur: number;
  outputCostPerMillionTokensEur: number;
  pricingUnit?: string;
  pricingSource?: string;
  requiresApproval: boolean;
  enabled: boolean;
  budgetUsedEur: number;
  inputTokensUsed: number;
  outputTokensUsed: number;
}

export interface ILLMRouteRequest {
  task: string;
  taskType?: string;
  difficulty?: number;
  requiredReasoning?: string;
  validationPassed?: boolean;
  previousModelId?: string;
}

export interface ILLMTaskClassification {
  taskType: string;
  difficulty: number;
  requiredReasoning: string;
  requiredCapabilities: string[];
  reason: string;
}

export interface ILLMFallbackOption {
  providerId: string;
  modelId: string;
  modelName: string;
  tier: string;
  estimatedCostEur: number;
  estimatedInputTokens: number;
  estimatedOutputTokens: number;
  requiresApproval: boolean;
}

export interface ILLMSkippedModel {
  providerId: string;
  modelId: string;
  reason: string;
}

export interface ILLMRouteDecision {
  selectedProviderId: string;
  selectedModelId: string;
  selectedModelName: string;
  tier: string;
  reason: string;
  estimatedCostEur: number;
  estimatedInputTokens: number;
  estimatedOutputTokens: number;
  pricingSource?: string;
  requiresApproval: boolean;
  classification: ILLMTaskClassification;
  fallbackPath: ILLMFallbackOption[];
  skipped: ILLMSkippedModel[];
  loggedAt: string;
}

export interface ILLMGenerationResult {
  providerId: string;
  modelId: string;
  modelName: string;
  tier: string;
  output: string;
  status: string;
  reason: string;
  estimatedCostEur: number;
  durationMs: number;
  fallbackPath: string[];
  loggedAt: string;
}
