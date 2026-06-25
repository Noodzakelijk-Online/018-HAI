export interface IAmbientPolicy {
  schedulerEnabled: boolean;
  suggestionOnly: boolean;
  executionEnabled: boolean;
  minimumScore: number;
  minimumConfidence: number;
  opportunityLimit: number;
  executionLimit: number;
  cooldownHours: number;
  scanIntervalSeconds: number;
  scanRetention: number;
  dualPathKvCacheMode: string;
  dualPathInfrastructureAvailable: boolean;
}

export interface IAmbientNeed {
  id: string;
  key: string;
  name: string;
  description: string;
  currentLevel: number;
  targetLevel: number;
  priorityWeight: number;
  enabled: boolean;
  notes?: string;
  updatedAt: string;
}

export interface IAmbientOpportunity {
  id: string;
  workflowId?: string;
  needKey: string;
  title: string;
  rationale: string;
  nextAction: string;
  sourceType?: string;
  sourceId?: string;
  sourceUri?: string;
  evidenceManifest?: string;
  resolutionNote?: string;
  priorityScore: number;
  urgency: number;
  impact: number;
  effort: number;
  confidence: number;
  risk: number;
  requiresApproval: boolean;
  status: string;
  lastSeenAt: string;
  cooldownUntil?: string;
}

export interface IAmbientScan {
  id: string;
  trigger: string;
  status: string;
  startedAt: string;
  completedAt?: string;
  itemsExamined: number;
  opportunitiesFound: number;
  created: number;
  updated: number;
  deduplicated: number;
  advanced: number;
  filtered: number;
  skipped: number;
  blocked: number;
  manifestBytes: number;
  deduplicatedBytes: number;
  errorMessage?: string;
}

export interface IAmbientOverview {
  generatedAt: string;
  policy: IAmbientPolicy;
  needs: IAmbientNeed[];
  opportunities: IAmbientOpportunity[];
  scans: IAmbientScan[];
  warnings: string[];
}

export interface IAmbientNeedUpdate {
  currentLevel: number;
  targetLevel: number;
  priorityWeight: number;
  enabled: boolean;
  notes?: string;
}
