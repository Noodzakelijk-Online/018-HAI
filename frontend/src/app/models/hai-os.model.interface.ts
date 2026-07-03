export interface IHAIOSMetric {
  label: string;
  value: number;
  status: string;
}

export interface IHAIOSPlane {
  name: string;
  status: string;
  description: string;
  links: string[];
}

export interface IHAIOSReferenceStack {
  name: string;
  status: string;
  use: string;
}

export interface IHAIOSReadinessGate {
  name: string;
  status: string;
  evidence: string;
  next: string;
}

export interface IHAIOSPursuitQueue {
  name: string;
  description: string;
  count: number;
  status: string;
  route: string;
}

export interface IHAIOSPursuitSpotlight {
  id: string;
  title: string;
  status: string;
  riskLevel: string;
  nextAction?: string;
  currentState?: string;
  evidenceLine?: string;
  needsRobert: number;
  blocked: number;
  openLoops: number;
  decisionCards: number;
  linkedEvidence: number;
  timelineItems: number;
  stale: boolean;
  reviewDue: boolean;
  planningNeeded: boolean;
}

export interface IHAIOSPursuitOverview {
  enabled: boolean;
  status: string;
  totalActive: number;
  needsRobert: number;
  vaReady: number;
  systemReady: number;
  blocked: number;
  stale: number;
  reviewDue: number;
  planningNeeded: number;
  highRisk: number;
  completionCandidates: number;
  decisionCards: number;
  linkedEvidence: number;
  openLoops: number;
  timelineItems: number;
  evidenceStatus: string;
  ambientProposals: number;
  ambientApprovalQueue: number;
  ambientLastScan?: string;
  ambientLine?: string;
  summary: string;
  next: string;
  queues: IHAIOSPursuitQueue[];
  spotlight: IHAIOSPursuitSpotlight[];
}

export interface IHAIOSOverview {
  generatedAt: string;
  canonicalStack: string;
  referenceStacks: IHAIOSReferenceStack[];
  localFirst: boolean;
  completionFirst: boolean;
  paidBudgetEur: number;
  paidUsageAllowed: boolean;
  metrics: IHAIOSMetric[];
  planes: IHAIOSPlane[];
  readinessGates: IHAIOSReadinessGate[];
  pursuitOverview: IHAIOSPursuitOverview;
  needsReviewTotal: number;
  emergencyStop: boolean;
  emergencyStopReason: string;
  emergencyStopNote: string;
}
