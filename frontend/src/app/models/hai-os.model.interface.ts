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
  needsReviewTotal: number;
  emergencyStopNote: string;
}
