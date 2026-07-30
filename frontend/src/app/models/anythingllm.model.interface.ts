export interface IAnythingLLMStatus {
  enabled: boolean
  configured: boolean
  provider: string
  endpoint?: string
  workspaceCount: number
  workspaceSlugs: string[]
  localEmbeddingsConfirmed: boolean
  configError?: string
  capabilities: string[]
  restrictions: string[]
  scope: string
}

export interface IAnythingLLMResult {
  chunkId: string
  workspaceSlug: string
  title?: string
  content: string
  sourceUri?: string
  score?: number
  distance?: number
}

export interface IAnythingLLMResponse {
  query: string
  workspaceSlug: string
  results: IAnythingLLMResult[]
  total: number
  scope: string
}
