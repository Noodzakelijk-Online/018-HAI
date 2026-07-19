export interface IRAGFlowStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  datasetCount: number
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}
