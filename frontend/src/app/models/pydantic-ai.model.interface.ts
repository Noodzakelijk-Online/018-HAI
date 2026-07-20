export interface IPydanticAIProposal {
  goal: string
  successCriteria: string[]
  nextSteps: string[]
  risk: 'low' | 'medium' | 'high'
  requiresApproval: boolean
  reasons: string[]
  uncertainties: string[]
}

export interface IPydanticAIResponse {
  engine: string
  modelId: string
  requestDigest: string
  proposal: IPydanticAIProposal
  scope: string
}
