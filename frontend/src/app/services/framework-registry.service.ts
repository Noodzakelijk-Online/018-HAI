import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable, map } from 'rxjs';
import {
  IActivateConstitutionRequest,
  IConstitution,
  IConstitutionDraftRequest,
  IConstitutionHistoryEntry,
  IConstitutionHistoryPage,
  IConstitutionSnapshot,
  FrameworkLifecycleStatus,
  IFrameworkPreferencePatch,
  IFrameworkRegistryOverview,
  IFrameworkSelectionDecision,
  IFrameworkSelectionRequest,
  IFrameworkView,
} from '../models/framework-registry.model.interface';

type FrameworkListResponse =
  | IFrameworkView[]
  | { frameworks: IFrameworkView[] };

type FrameworkResponse =
  | IFrameworkView
  | { framework: IFrameworkView };

type SelectionResponse =
  | IFrameworkSelectionDecision
  | { selection: IFrameworkSelectionDecision };

type SelectionListResponse =
  | IFrameworkSelectionDecision[]
  | { selections: IFrameworkSelectionDecision[] };

type ActiveConstitutionResponse =
  | IConstitution
  | { constitution: IConstitution; source?: string };

type ConstitutionMutationResponse =
  | IConstitution
  | { constitution: IConstitution }
  | { draft: IConstitution };

@Injectable({ providedIn: 'root' })
export class FrameworkRegistryService {
  private readonly apiUrl = '/api/v1/framework-registry';

  constructor(private http: HttpClient) {}

  overview(): Observable<IFrameworkRegistryOverview> {
    return this.http.get<IFrameworkRegistryOverview>(`${this.apiUrl}/overview`).pipe(
      map((response) => this.normalizeOverview(response))
    );
  }

  frameworks(): Observable<IFrameworkView[]> {
    return this.http.get<FrameworkListResponse>(`${this.apiUrl}/frameworks`).pipe(
      map((response) => this.normalizeFrameworkList(response))
    );
  }

  framework(id: string): Observable<IFrameworkView> {
    return this.http.get<FrameworkResponse>(
      `${this.apiUrl}/frameworks/${encodeURIComponent(id)}`
    ).pipe(
      map((response) => this.normalizeFramework(
        this.unwrapRecord(response, 'framework'),
        'framework'
      ))
    );
  }

  select(request: IFrameworkSelectionRequest): Observable<IFrameworkSelectionDecision> {
    return this.http.post<SelectionResponse>(
      `${this.apiUrl}/select`,
      this.safeSelectionRequest(request)
    ).pipe(
      map((response) => this.normalizeSelection(
        this.unwrapRecord(response, 'selection'),
        'selection'
      ))
    );
  }

  updatePreference(
    id: string,
    preference: IFrameworkPreferencePatch
  ): Observable<IFrameworkView> {
    return this.http.patch<FrameworkResponse>(
      `${this.apiUrl}/frameworks/${encodeURIComponent(id)}/preference`,
      this.safePreferencePatch(preference)
    ).pipe(
      map((response) => this.normalizeFramework(
        this.unwrapRecord(response, 'framework'),
        'framework preference'
      ))
    );
  }

  selections(): Observable<IFrameworkSelectionDecision[]> {
    return this.http.get<SelectionListResponse>(`${this.apiUrl}/selections`).pipe(
      map((response) => this.normalizeSelectionList(response))
    );
  }

  constitution(): Observable<IConstitutionSnapshot> {
    return this.http.get<ActiveConstitutionResponse>(`${this.apiUrl}/constitution`).pipe(
      map((response) => this.unwrapActiveConstitution(response))
    );
  }

  constitutionHistory(limit = 50): Observable<IConstitutionHistoryPage> {
    const boundedLimit = Number.isInteger(limit) && limit > 0 && limit <= 100
      ? limit
      : 50;
    return this.http.get<IConstitutionHistoryPage>(
      `${this.apiUrl}/constitution/history`,
      { params: new HttpParams().set('limit', boundedLimit) }
    ).pipe(map((response) => this.normalizeConstitutionHistory(response)));
  }

  createConstitutionDraft(request: IConstitutionDraftRequest): Observable<IConstitution> {
    return this.http.post<ConstitutionMutationResponse>(
      `${this.apiUrl}/constitution/drafts`,
      this.safeConstitutionDraft(request)
    ).pipe(map((response) => this.unwrapConstitution(response)));
  }

  activateConstitution(
    id: string,
    request: IActivateConstitutionRequest
  ): Observable<IConstitution> {
    return this.http.post<ConstitutionMutationResponse>(
      `${this.apiUrl}/constitution/${encodeURIComponent(id)}/activate`,
      {
        confirmation: request.confirmation,
        approvalNote: request.approvalNote,
      }
    ).pipe(map((response) => this.unwrapConstitution(response)));
  }

  private unwrapActiveConstitution(
    response: ActiveConstitutionResponse
  ): IConstitutionSnapshot {
    const sanitized = this.sanitizeInbound(response) as unknown;
    if (this.isRecord(sanitized) && 'constitution' in sanitized) {
      return {
        constitution: this.normalizeConstitution(
          sanitized['constitution'],
          'active Constitution'
        ),
        source: sanitized['source'] === undefined
          ? ''
          : this.requireString(sanitized, 'source', 'active Constitution'),
      };
    }
    return {
      constitution: this.normalizeConstitution(sanitized, 'active Constitution'),
      source: '',
    };
  }

  private unwrapConstitution(response: ConstitutionMutationResponse): IConstitution {
    const sanitized = this.sanitizeInbound(response) as unknown;
    if (this.isRecord(sanitized) && 'constitution' in sanitized) {
      return this.normalizeConstitution(
        sanitized['constitution'],
        'Constitution mutation'
      );
    }
    if (this.isRecord(sanitized) && 'draft' in sanitized) {
      return this.normalizeConstitution(
        sanitized['draft'],
        'Constitution draft'
      );
    }
    return this.normalizeConstitution(sanitized, 'Constitution mutation');
  }

  private normalizeOverview(value: unknown): IFrameworkRegistryOverview {
    const record = this.requireRecord(
      this.sanitizeInbound(value),
      'registry overview'
    );
    const familiesRecord = this.requireRecord(
      record['families'],
      'registry overview families'
    );
    const families: Record<string, number> = {};
    Object.entries(familiesRecord).forEach(([family, count]) => {
      if (!Number.isInteger(count) || (count as number) < 0) {
        throw this.contractError('registry overview families');
      }
      families[family] = count as number;
    });
    return {
      generatedAt: this.requireDateString(record, 'generatedAt', 'registry overview'),
      total: this.requireInteger(record, 'total', 'registry overview'),
      enabled: this.requireInteger(record, 'enabled', 'registry overview'),
      experimental: this.requireInteger(record, 'experimental', 'registry overview'),
      deprecated: this.requireInteger(record, 'deprecated', 'registry overview'),
      pinned: this.requireInteger(record, 'pinned', 'registry overview'),
      families,
      constitutionVersion: this.requireInteger(
        record,
        'constitutionVersion',
        'registry overview',
        1
      ),
      constitutionSource: this.requireString(
        record,
        'constitutionSource',
        'registry overview'
      ),
      recentSelections: this.requireInteger(
        record,
        'recentSelections',
        'registry overview'
      ),
      selectionContract: this.requireStringArray(
        record,
        'selectionContract',
        'registry overview'
      ),
    };
  }

  private normalizeFrameworkList(value: unknown): IFrameworkView[] {
    const sanitized = this.sanitizeInbound(value) as unknown;
    const payload = Array.isArray(sanitized)
      ? sanitized
      : this.isRecord(sanitized)
        ? sanitized['frameworks']
        : undefined;
    if (!Array.isArray(payload)) {
      throw this.contractError('framework list');
    }
    return payload.map((entry, index) =>
      this.normalizeFramework(entry, `framework list item ${index + 1}`)
    );
  }

  private normalizeFramework(value: unknown, resource: string): IFrameworkView {
    const record = this.requireRecord(this.sanitizeInbound(value), resource);
    const status = this.requireEnum<FrameworkLifecycleStatus>(
      record,
      'status',
      resource,
      ['active', 'experimental', 'deprecated']
    );
    const result: IFrameworkView = {
      id: this.requireString(record, 'id', resource),
      version: this.requireString(record, 'version', resource),
      name: this.requireString(record, 'name', resource),
      family: this.requireString(record, 'family', resource),
      purpose: this.requireString(record, 'purpose', resource),
      suitableProblemTypes: this.requireStringArray(record, 'suitableProblemTypes', resource),
      triggerConditions: this.requireStringArray(record, 'triggerConditions', resource),
      requiredInputs: this.requireStringArray(record, 'requiredInputs', resource),
      producedOutputs: this.requireStringArray(record, 'producedOutputs', resource),
      requiredAgents: this.requireStringArray(record, 'requiredAgents', resource),
      workflowTemplate: this.requireStringArray(record, 'workflowTemplate', resource),
      decisionRules: this.requireStringArray(record, 'decisionRules', resource),
      safetyInvariants: this.requireStringArray(record, 'safetyInvariants', resource),
      authorityRequirement: this.requireString(record, 'authorityRequirement', resource),
      maximumAutonomyLevel: this.requireInteger(
        record,
        'maximumAutonomyLevel',
        resource,
        0,
        10
      ),
      riskCeiling: this.requireString(record, 'riskCeiling', resource),
      evidenceRequirements: this.requireStringArray(record, 'evidenceRequirements', resource),
      evaluationMethod: this.requireStringArray(record, 'evaluationMethod', resource),
      conflictsWith: this.requireStringArray(record, 'conflictsWith', resource),
      userSpecificAdaptations: this.requireStringArray(
        record,
        'userSpecificAdaptations',
        resource
      ),
      source: this.requireString(record, 'source', resource),
      provenance: this.requireString(record, 'provenance', resource),
      status,
      effectiveStatus: this.requireString(record, 'effectiveStatus', resource),
      enabled: this.requireBoolean(record, 'enabled', resource),
      pinned: this.requireBoolean(record, 'pinned', resource),
      effectiveAutonomyLevel: this.requireInteger(
        record,
        'effectiveAutonomyLevel',
        resource,
        0,
        10
      ),
      adaptations: this.requireStringArray(record, 'adaptations', resource),
    };
    if (record['candidateImplementations'] !== undefined) {
      result.candidateImplementations = this.requireStringArray(
        record,
        'candidateImplementations',
        resource
      );
    }
    if (record['preferenceUpdatedAt'] !== undefined) {
      result.preferenceUpdatedAt = this.requireDateString(
        record,
        'preferenceUpdatedAt',
        resource
      );
    }
    return result;
  }

  private normalizeSelectionList(value: unknown): IFrameworkSelectionDecision[] {
    const sanitized = this.sanitizeInbound(value) as unknown;
    const payload = Array.isArray(sanitized)
      ? sanitized
      : this.isRecord(sanitized)
        ? sanitized['selections']
        : undefined;
    if (!Array.isArray(payload)) {
      throw this.contractError('selection history');
    }
    return payload.map((entry, index) =>
      this.normalizeSelection(entry, `selection history item ${index + 1}`)
    );
  }

  private normalizeSelection(
    value: unknown,
    resource: string
  ): IFrameworkSelectionDecision {
    const record = this.requireRecord(this.sanitizeInbound(value), resource);
    const selected = this.requireArray(record, 'selected', resource).map(
      (entry, index) => {
        const itemResource = `${resource} selected framework ${index + 1}`;
        const item = this.requireRecord(entry, itemResource);
        return {
          id: this.requireString(item, 'id', itemResource),
          version: this.requireString(item, 'version', itemResource),
          name: this.requireString(item, 'name', itemResource),
          family: this.requireString(item, 'family', itemResource),
          score: this.requireNumber(item, 'score', itemResource),
          reasons: this.requireStringArray(item, 'reasons', itemResource),
          maximumAutonomyLevel: this.requireInteger(
            item,
            'maximumAutonomyLevel',
            itemResource,
            0,
            10
          ),
          authorityRequirement: this.requireString(
            item,
            'authorityRequirement',
            itemResource
          ),
          evidenceRequirements: this.requireStringArray(
            item,
            'evidenceRequirements',
            itemResource
          ),
          evaluationMethod: this.requireStringArray(
            item,
            'evaluationMethod',
            itemResource
          ),
        };
      }
    );
    const conflicts = this.requireArray(record, 'conflicts', resource).map(
      (entry, index) => {
        const itemResource = `${resource} conflict ${index + 1}`;
        const item = this.requireRecord(entry, itemResource);
        return {
          selectedId: this.requireString(item, 'selectedId', itemResource),
          skippedId: this.requireString(item, 'skippedId', itemResource),
          reason: this.requireString(item, 'reason', itemResource),
        };
      }
    );
    const result: IFrameworkSelectionDecision = {
      id: this.requireString(record, 'id', resource),
      createdAt: this.requireDateString(record, 'createdAt', resource),
      catalogVersion: this.requireString(record, 'catalogVersion', resource),
      catalogDigest: this.requireDigest(record, 'catalogDigest', resource),
      selectorAlgorithmVersion: this.requireString(
        record,
        'selectorAlgorithmVersion',
        resource
      ),
      effectivePreferenceDigest: this.requireDigest(
        record,
        'effectivePreferenceDigest',
        resource
      ),
      constitutionDigest: this.requireDigest(record, 'constitutionDigest', resource),
      lifeDomain: this.requireString(record, 'lifeDomain', resource),
      needOrCommitment: this.requireString(record, 'needOrCommitment', resource),
      selected,
      conflicts,
      requiredAgents: this.requireStringArray(record, 'requiredAgents', resource),
      maximumAutonomyLevel: this.requireInteger(
        record,
        'maximumAutonomyLevel',
        resource,
        0,
        10
      ),
      authoritySummary: this.requireString(record, 'authoritySummary', resource),
      requiresApproval: this.requireBoolean(record, 'requiresApproval', resource),
      approvalReasons: this.requireStringArray(record, 'approvalReasons', resource),
      evidenceRequirements: this.requireStringArray(
        record,
        'evidenceRequirements',
        resource
      ),
      completionCriteria: this.requireStringArray(
        record,
        'completionCriteria',
        resource
      ),
      learningPlan: this.requireStringArray(record, 'learningPlan', resource),
      contextRequirements: this.requireStringArray(
        record,
        'contextRequirements',
        resource
      ),
      selectionReason: this.requireString(record, 'selectionReason', resource),
      constitutionVersion: this.requireInteger(
        record,
        'constitutionVersion',
        resource,
        1
      ),
      constitutionSource: this.requireString(
        record,
        'constitutionSource',
        resource
      ),
    };
    if (record['taskPlanId'] !== undefined) {
      result.taskPlanId = this.requireString(record, 'taskPlanId', resource, true);
    }
    return result;
  }

  private normalizeConstitution(value: unknown, resource: string): IConstitution {
    const record = this.requireRecord(this.sanitizeInbound(value), resource);
    const result: IConstitution = {
      id: this.requireString(record, 'id', resource),
      version: this.requireInteger(record, 'version', resource, 1),
      baseVersion: this.requireInteger(record, 'baseVersion', resource, 0),
      status: this.requireEnum(
        record,
        'status',
        resource,
        ['draft', 'active', 'superseded'] as const
      ),
      values: this.requireStringArray(record, 'values', resource),
      prohibitions: this.requireStringArray(record, 'prohibitions', resource),
      standingPermissions: this.requireStringArray(
        record,
        'standingPermissions',
        resource
      ),
      preferences: this.requireStringArray(record, 'preferences', resource),
      relationshipRules: this.requireStringArray(
        record,
        'relationshipRules',
        resource
      ),
      financialBoundaries: this.requireStringArray(
        record,
        'financialBoundaries',
        resource
      ),
      communicationRules: this.requireStringArray(
        record,
        'communicationRules',
        resource
      ),
      escalationRules: this.requireStringArray(record, 'escalationRules', resource),
      protectedRules: this.requireStringArray(record, 'protectedRules', resource),
      createdAt: this.requireDateString(record, 'createdAt', resource),
    };
    for (const key of ['changeSummary', 'approvedBy'] as const) {
      if (record[key] !== undefined) {
        result[key] = this.requireString(record, key, resource, true);
      }
    }
    if (record['approvedAt'] !== undefined) {
      result.approvedAt = this.requireDateString(record, 'approvedAt', resource);
    }
    return result;
  }

  private normalizeConstitutionHistory(value: unknown): IConstitutionHistoryPage {
    const record = this.requireRecord(
      this.sanitizeInbound(value),
      'Constitution history'
    );
    const history = this.requireArray(
      record,
      'history',
      'Constitution history'
    ).map((entry, index) =>
      this.normalizeConstitutionHistoryEntry(
        entry,
        `Constitution history item ${index + 1}`
      )
    );
    return {
      history,
      limit: this.requireInteger(record, 'limit', 'Constitution history', 1, 100),
      truncated: this.requireBoolean(record, 'truncated', 'Constitution history'),
    };
  }

  private normalizeConstitutionHistoryEntry(
    value: unknown,
    resource: string
  ): IConstitutionHistoryEntry {
    const record = this.requireRecord(this.sanitizeInbound(value), resource);
    const result: IConstitutionHistoryEntry = {
      id: this.requireString(record, 'id', resource),
      version: this.requireInteger(record, 'version', resource, 1),
      baseVersion: this.requireInteger(record, 'baseVersion', resource, 0),
      status: this.requireEnum(
        record,
        'status',
        resource,
        ['draft', 'active', 'superseded'] as const
      ),
      changeSummary: this.requireString(record, 'changeSummary', resource, true),
      createdAt: this.requireDateString(record, 'createdAt', resource),
      digest: this.requireDigest(record, 'digest', resource),
    };
    if (record['approvedBy'] !== undefined) {
      result.approvedBy = this.requireString(record, 'approvedBy', resource, true);
    }
    if (record['approvedAt'] !== undefined) {
      result.approvedAt = this.requireDateString(record, 'approvedAt', resource);
    }
    return result;
  }

  private unwrapRecord(value: unknown, key: string): unknown {
    const sanitized = this.sanitizeInbound(value) as unknown;
    return this.isRecord(sanitized) && key in sanitized
      ? sanitized[key]
      : sanitized;
  }

  private safePreferencePatch(
    preference: IFrameworkPreferencePatch
  ): IFrameworkPreferencePatch {
    return {
      state: preference.state,
      ...(preference.pinned !== undefined ? { pinned: preference.pinned } : {}),
      ...(preference.maximumAutonomyLevel !== undefined
        ? { maximumAutonomyLevel: preference.maximumAutonomyLevel }
        : {}),
      ...(preference.clearAutonomyOverride !== undefined
        ? { clearAutonomyOverride: preference.clearAutonomyOverride }
        : {}),
      ...(preference.adaptations !== undefined
        ? { adaptations: preference.adaptations }
        : {}),
    };
  }

  private safeConstitutionDraft(
    request: IConstitutionDraftRequest
  ): IConstitutionDraftRequest {
    return {
      ...(request.baseVersion !== undefined
        ? { baseVersion: request.baseVersion }
        : {}),
      values: request.values,
      prohibitions: request.prohibitions,
      standingPermissions: request.standingPermissions,
      preferences: request.preferences,
      relationshipRules: request.relationshipRules,
      financialBoundaries: request.financialBoundaries,
      communicationRules: request.communicationRules,
      escalationRules: request.escalationRules,
      changeSummary: request.changeSummary,
    };
  }

  private requireRecord(value: unknown, resource: string): Record<string, unknown> {
    if (!this.isRecord(value)) {
      throw this.contractError(resource);
    }
    return value;
  }

  private isRecord(value: unknown): value is Record<string, unknown> {
    return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
  }

  private requireString(
    record: Record<string, unknown>,
    key: string,
    resource: string,
    allowEmpty = false
  ): string {
    const value = record[key];
    if (
      typeof value !== 'string' ||
      (!allowEmpty && value.trim().length === 0)
    ) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireDateString(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): string {
    const value = this.requireString(record, key, resource);
    if (!Number.isFinite(Date.parse(value))) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireNumber(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): number {
    const value = record[key];
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireInteger(
    record: Record<string, unknown>,
    key: string,
    resource: string,
    minimum = 0,
    maximum = Number.MAX_SAFE_INTEGER
  ): number {
    const value = this.requireNumber(record, key, resource);
    if (!Number.isInteger(value) || value < minimum || value > maximum) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireBoolean(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): boolean {
    const value = record[key];
    if (typeof value !== 'boolean') {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireArray(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): unknown[] {
    const value = record[key];
    if (!Array.isArray(value)) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireStringArray(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): string[] {
    const value = this.requireArray(record, key, resource);
    if (!value.every((entry) => typeof entry === 'string')) {
      throw this.contractError(resource);
    }
    return value as string[];
  }

  private requireDigest(
    record: Record<string, unknown>,
    key: string,
    resource: string
  ): string {
    const value = this.requireString(record, key, resource);
    if (!/^[0-9a-f]{64}$/.test(value)) {
      throw this.contractError(resource);
    }
    return value;
  }

  private requireEnum<T extends string>(
    record: Record<string, unknown>,
    key: string,
    resource: string,
    allowed: readonly T[]
  ): T {
    const value = this.requireString(record, key, resource);
    if (!allowed.includes(value as T)) {
      throw this.contractError(resource);
    }
    return value as T;
  }

  private contractError(resource: string): Error {
    return new Error(`Invalid Framework Registry ${resource} response.`);
  }

  private sanitizeInbound<T>(value: T, key = ''): T {
    if (this.isSensitiveKey(key)) {
      return '[redacted]' as T;
    }
    if (typeof value === 'string') {
      return this.redactSensitiveText(value) as T;
    }
    if (Array.isArray(value)) {
      return value.map((entry) => this.sanitizeInbound(entry, key)) as T;
    }
    if (value && typeof value === 'object') {
      return Object.fromEntries(
        Object.entries(value as Record<string, unknown>).map(([key, entry]) => [
          key,
          this.sanitizeInbound(entry, key),
        ])
      ) as T;
    }
    return value;
  }

  private isSensitiveKey(key: string): boolean {
    const normalized = key.replace(/[^a-z0-9]/gi, '').toLowerCase();
    return [
      'password',
      'passwd',
      'secret',
      'apikey',
      'accesstoken',
      'refreshtoken',
      'authorization',
      'cookie',
      'clientsecret',
      'privatekey',
      'token',
      'idtoken',
      'sessiontoken',
      'credential',
      'credentials',
      'passphrase',
      'encryptionkey',
    ].includes(normalized);
  }

  private redactSensitiveText(value: string): string {
    return value
      .replace(
        /(\bAuthorization\b\s*:\s*)(?:Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+/gi,
        '$1[redacted]'
      )
      .replace(/\bBearer\s+[A-Za-z0-9._~+/=-]+/gi, 'Bearer [redacted]')
      .replace(
        /(\b(?:password|passwd|secret|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|authorization|cookie)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)/gi,
        '$1[redacted]'
      )
      .replace(
        /([?&](?:password|secret|api[_-]?key|access[_-]?token|refresh[_-]?token|token)=)[^&#\s]+/gi,
        '$1[redacted]'
      )
      .replace(
        /\b(?:sk|ghp|gho|github_pat|glpat|xox[baprs])[-_][A-Za-z0-9_-]{12,}\b/g,
        '[redacted]'
      )
      .replace(
        /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g,
        '[redacted]'
      );
  }

  private safeSelectionRequest(
    request: IFrameworkSelectionRequest
  ): IFrameworkSelectionRequest {
    return {
      request: request.request,
      ...(request.taskPlanId !== undefined ? { taskPlanId: request.taskPlanId } : {}),
      ...(request.projectKey !== undefined ? { projectKey: request.projectKey } : {}),
      ...(request.pursuitId !== undefined ? { pursuitId: request.pursuitId } : {}),
      ...(request.taskType !== undefined ? { taskType: request.taskType } : {}),
      ...(request.difficulty !== undefined ? { difficulty: request.difficulty } : {}),
      ...(request.requiredReasoning !== undefined
        ? { requiredReasoning: request.requiredReasoning }
        : {}),
      ...(request.successCriteria !== undefined
        ? { successCriteria: request.successCriteria }
        : {}),
      ...(request.needsMemory !== undefined ? { needsMemory: request.needsMemory } : {}),
      ...(request.needsTools !== undefined ? { needsTools: request.needsTools } : {}),
      ...(request.needsDocuments !== undefined
        ? { needsDocuments: request.needsDocuments }
        : {}),
      ...(request.needsWebAccess !== undefined
        ? { needsWebAccess: request.needsWebAccess }
        : {}),
      ...(request.needsLocalExecution !== undefined
        ? { needsLocalExecution: request.needsLocalExecution }
        : {}),
      ...(request.executeRequested !== undefined
        ? { executeRequested: request.executeRequested }
        : {}),
    };
  }
}
