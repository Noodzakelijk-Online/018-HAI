export interface ISyftStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  workspaces: string[]
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}
