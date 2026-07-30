export type FrameworkLifecycleStatus = 'active' | 'experimental' | 'deprecated';
export type FrameworkPreferenceState = 'default' | 'enabled' | 'disabled';
export type FrameworkViewMode = 'basic' | 'advanced';

export const PROTECTED_FRAMEWORK_IDS = [
  'human-sovereignty',
  'intake-triage',
  'autonomy-levels',
  'approval-control',
  'truth-evidence',
  'privacy-protection',
  'security-zero-trust',
  'agent-threat-modeling',
  'reliable-execution',
  'evaluation',
] as const;

export function isProtectedFrameworkId(id: string): boolean {
  const normalizedId = id.trim();
  return PROTECTED_FRAMEWORK_IDS.some((protectedId) => protectedId === normalizedId);
}

export interface IFramework {
  id: string;
  version: string;
  name: string;
  family: string;
  purpose: string;
  suitableProblemTypes: string[];
  triggerConditions: string[];
  requiredInputs: string[];
  producedOutputs: string[];
  requiredAgents: string[];
  workflowTemplate: string[];
  decisionRules: string[];
  safetyInvariants: string[];
  authorityRequirement: string;
  maximumAutonomyLevel: number;
  riskCeiling: string;
  evidenceRequirements: string[];
  evaluationMethod: string[];
  conflictsWith: string[];
  userSpecificAdaptations: string[];
  candidateImplementations?: string[];
  source: string;
  provenance: string;
  status: FrameworkLifecycleStatus;
}

export interface IFrameworkPreference {
  frameworkId: string;
  state: FrameworkPreferenceState;
  pinned: boolean;
  maximumAutonomyLevel?: number;
  adaptations: string[];
  updatedAt: string;
}

export interface IFrameworkView extends IFramework {
  effectiveStatus: string;
  enabled: boolean;
  pinned: boolean;
  effectiveAutonomyLevel: number;
  adaptations: string[];
  preferenceUpdatedAt?: string;
}

export interface IFrameworkPreferencePatch {
  state: FrameworkPreferenceState;
  pinned?: boolean;
  maximumAutonomyLevel?: number;
  clearAutonomyOverride?: boolean;
  adaptations?: string[];
}

export interface IFrameworkSelectionRequest {
  taskPlanId?: string;
  request: string;
  projectKey?: string;
  pursuitId?: string;
  taskType?: string;
  difficulty?: number;
  requiredReasoning?: string;
  successCriteria?: string[];
  needsMemory?: boolean;
  needsTools?: boolean;
  needsDocuments?: boolean;
  needsWebAccess?: boolean;
  needsLocalExecution?: boolean;
  executeRequested?: boolean;
}

export interface ISelectedFramework {
  id: string;
  version: string;
  name: string;
  family: string;
  score: number;
  reasons: string[];
  maximumAutonomyLevel: number;
  authorityRequirement: string;
  evidenceRequirements: string[];
  evaluationMethod: string[];
}

export interface IFrameworkConflict {
  selectedId: string;
  skippedId: string;
  reason: string;
}

export interface IFrameworkSelectionDecision {
  id: string;
  taskPlanId?: string;
  createdAt: string;
  catalogVersion: string;
  catalogDigest: string;
  selectorAlgorithmVersion: string;
  effectivePreferenceDigest: string;
  constitutionDigest: string;
  lifeDomain: string;
  needOrCommitment: string;
  selected: ISelectedFramework[];
  conflicts: IFrameworkConflict[];
  requiredAgents: string[];
  maximumAutonomyLevel: number;
  authoritySummary: string;
  requiresApproval: boolean;
  approvalReasons: string[];
  evidenceRequirements: string[];
  completionCriteria: string[];
  learningPlan: string[];
  contextRequirements: string[];
  selectionReason: string;
  constitutionVersion: number;
  constitutionSource: string;
}

export interface IConstitution {
  id: string;
  version: number;
  baseVersion: number;
  status: 'draft' | 'active' | 'superseded';
  values: string[];
  prohibitions: string[];
  standingPermissions: string[];
  preferences: string[];
  relationshipRules: string[];
  financialBoundaries: string[];
  communicationRules: string[];
  escalationRules: string[];
  protectedRules: string[];
  changeSummary?: string;
  approvedBy?: string;
  approvedAt?: string;
  createdAt: string;
}

export interface IConstitutionHistoryEntry {
  id: string;
  version: number;
  baseVersion: number;
  status: 'draft' | 'active' | 'superseded';
  changeSummary: string;
  approvedBy?: string;
  approvedAt?: string;
  createdAt: string;
  digest: string;
}

export interface IConstitutionHistoryPage {
  history: IConstitutionHistoryEntry[];
  limit: number;
  truncated: boolean;
}

export interface IConstitutionSnapshot {
  constitution: IConstitution;
  source: string;
}

export interface IConstitutionDraftRequest {
  baseVersion?: number;
  values: string[];
  prohibitions: string[];
  standingPermissions: string[];
  preferences: string[];
  relationshipRules: string[];
  financialBoundaries: string[];
  communicationRules: string[];
  escalationRules: string[];
  changeSummary: string;
}

export interface IActivateConstitutionRequest {
  confirmation: string;
  approvalNote: string;
}

export interface IFrameworkRegistryOverview {
  generatedAt: string;
  total: number;
  enabled: number;
  experimental: number;
  deprecated: number;
  pinned: number;
  families: Record<string, number>;
  constitutionVersion: number;
  constitutionSource: string;
  recentSelections: number;
  selectionContract: string[];
}

export interface IFrameworkModuleViewPreferences {
  version: 1;
  mode: FrameworkViewMode;
  openSections: Record<string, boolean>;
}
