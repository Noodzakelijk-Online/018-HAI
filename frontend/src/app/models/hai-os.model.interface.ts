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

export interface IHAIOSOverview {
  generatedAt: string;
  localFirst: boolean;
  completionFirst: boolean;
  paidBudgetEur: number;
  paidUsageAllowed: boolean;
  metrics: IHAIOSMetric[];
  planes: IHAIOSPlane[];
  needsReviewTotal: number;
  emergencyStopNote: string;
}
