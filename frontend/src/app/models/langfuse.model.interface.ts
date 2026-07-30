export interface ILangfuseStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}

export interface ILangfuseProbeResult {
  healthy: boolean
  ready: boolean
  checkedAt: string
  scope: string
}

export interface ILangfuseExportResult {
  traceId: string
  spanId: string
  exportedAt: string
  scope: string
}
