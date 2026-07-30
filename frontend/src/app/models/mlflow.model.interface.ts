export interface IMLflowStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  experimentIds: string[]
  metricKeys: string[]
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}
