export interface IOpenLITStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}

export interface IOpenLITExportResult {
  traceId: string
  spanId: string
  exportedAt: string
  scope: string
}
