export interface IResearchStatus {
  enabled: boolean;
  configured: boolean;
  provider: string;
  endpoint?: string;
  configError?: string;
  capabilities?: string[];
  restrictions?: string[];
  scope: string;
}

export interface IResearchProbe {
  reachable: boolean;
  checkedAt: string;
  scope: string;
}

export interface IResearchResult {
  title: string;
  sourceUri: string;
  snippet: string;
  engines?: string[];
  publishedAt?: string;
}

export interface IResearchResponse {
  query: string;
  results: IResearchResult[];
  scope: string;
}
