export interface IMCPPreflightTool {
  name: string;
  title?: string;
  hasInputSchema: boolean;
}

export interface IMCPPreflightResult {
  id: string;
  serverId: string;
  catalogId?: string;
  catalogName?: string;
  url?: string;
  status: 'ready' | 'disabled' | 'blocked' | 'failed';
  detail: string;
  protocolVersion?: string;
  toolCount: number;
  tools?: IMCPPreflightTool[];
  truncated: boolean;
  durationMs: number;
  checkedAt: string;
}

export interface IMCPPreflightServer {
  id: string;
  catalogId?: string;
  catalogName?: string;
  url?: string;
  configured: boolean;
  lastAttempt?: IMCPPreflightResult;
}

export interface IMCPPreflightOverview {
  enabled: boolean;
  configError?: string;
  scope: string;
  servers: IMCPPreflightServer[];
}
