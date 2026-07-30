export interface IPresidioStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  language: string
  entityTypes: string[]
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}
