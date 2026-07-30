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

export interface IRAGFlowResult {
  chunkId: string
  datasetId: string
  documentId?: string
  documentName?: string
  content: string
  similarity?: number
  termSimilarity?: number
  vectorSimilarity?: number
}

export interface IRAGFlowResponse {
  query: string
  results: IRAGFlowResult[]
  total: number
  datasetIds: string[]
  scope: string
}

export interface IRAGFlowProbeResult {
  reachable: boolean
  checkedAt: string
  scope: string
}
