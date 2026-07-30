export interface ICrewAIProposal {
  goal: string;
  successCriteria: string[];
  nextSteps: string[];
  risk: 'low' | 'medium' | 'high';
  requiresApproval: boolean;
  reasons: string[];
  uncertainties: string[];
}

export interface ICrewAIResponse {
  engine: string;
  modelId: string;
  requestDigest: string;
  proposal: ICrewAIProposal;
  scope: string;
}
