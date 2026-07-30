export interface ISerenaStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  projectId?: string
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}
