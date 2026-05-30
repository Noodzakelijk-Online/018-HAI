export interface IAutomationModel {
  image: string;
  name: string;
  urlPath?: string;
  port: number;
  position: number;
  host: string;
  id?: string;
  imageFile?: File;
  removeImage: boolean;

  launchType?: string;
  launchTarget?: string;
  runtimeType?: string;
  serviceName?: string;
  routePath?: string;
  publicUrl?: string;
  localUrl?: string;
  dependencyNotes?: string;
  healthCheckType?: string;
  healthCheckUrl?: string;
  healthCheckIntervalSeconds?: number;
  expectedHttpStatus?: number;
  status?: string;
  lastCheckedAt?: string;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  lastFailureReason?: string;
  consecutiveFailures?: number;
  averageLatencyMs?: number;
  lastLaunchAt?: string;
}

export interface IAutomationHealthSummary {
  total: number;
  healthy: number;
  warning: number;
  degraded: number;
  broken: number;
  unknown: number;
  checkedAt: string;
}

export interface IAutomationHealthResult {
  automationId: string;
  status: string;
  checkedAt: string;
  latencyMs: number;
  failureReason?: string;
  consecutiveFailures: number;
}

export interface IAutomationHealthEvent {
  id?: string;
  automationId: string;
  status: string;
  checkType: string;
  target?: string;
  latencyMs: number;
  failureReason?: string;
  consecutiveFailures: number;
  checkedAt: string;
}

export interface IAutomationDiagnostics {
  automationId: string;
  name: string;
  status: string;
  launchTarget: string;
  healthCheckTarget: string;
  routePath: string;
  host: string;
  port: number;
  lastCheckedAt?: string;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  lastFailureReason?: string;
  checks: Record<string, string>;
  recentEvents: IAutomationHealthEvent[];
}
