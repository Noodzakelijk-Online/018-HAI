export interface IAutonomyMetrics {
  attempts: number;
  rawCompletions: number;
  completionUnderPolicy: number;
  policyViolations: number;
  invalidActions: number;
  humanInterventions: number;
  recoveryAttempts: number;
  recovered: number;
  averageLatencyMillis: number;
  rawCompletionRate: number;
  policyCompletionRate: number;
  interventionRate: number;
  recoveryRate: number;
}

export interface IAutonomyActionTrace {
  id: string;
  workflowId: string;
  attempt: number;
  interfaceType: string;
  actionType: string;
  status: string;
  policyDecision: string;
  policyReason?: string;
  requiresApproval: boolean;
  approvalRecorded: boolean;
  executionVerified: boolean;
  verificationStatus?: string;
  latencyMilliseconds: number;
  resultSummary?: string;
  startedAt: string;
  completedAt?: string;
}

export interface IAutonomyEvaluation {
  id: string;
  workflowId: string;
  attempt: number;
  rawCompletion: boolean;
  executionBasedCorrectness: boolean;
  completionUnderPolicy: boolean;
  policyCompliant: boolean;
  humanIntervention: boolean;
  recoveryAttempt: boolean;
  recovered: boolean;
  failureMode?: string;
  latencyMilliseconds: number;
  createdAt: string;
}

export interface IAutonomyStressRun {
  id: string;
  passed: number;
  failed: number;
  createdAt: string;
}

export interface IAutonomyStressCase {
  name: string;
  passed: boolean;
  expected: string;
  actual: string;
}

export interface IAutonomyOverview {
  generatedAt: string;
  metrics: IAutonomyMetrics;
  recentActions: IAutonomyActionTrace[];
  recentEvaluations: IAutonomyEvaluation[];
  recentStressRuns: IAutonomyStressRun[];
  decisionDiscipline: {
    name: string;
    enabled: boolean;
    order: string[];
    newDependenciesDefault: string;
    benchmarkClaims: string;
  };
  warnings: string[];
}

export interface IAutonomyStressResult {
  run: IAutonomyStressRun;
  results: IAutonomyStressCase[];
}
