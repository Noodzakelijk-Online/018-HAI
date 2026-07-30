import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import {
  IConstitution,
  IConstitutionHistoryPage,
  IFrameworkRegistryOverview,
  IFrameworkSelectionDecision,
  IFrameworkSelectionRequest,
  IFrameworkView,
} from '../models/framework-registry.model.interface';
import { FrameworkRegistryService } from './framework-registry.service';

describe('FrameworkRegistryService', () => {
  let service: FrameworkRegistryService;
  let http: HttpTestingController;

  const framework: IFrameworkView = {
    id: 'truth-evidence',
    version: '1.0.0',
    name: 'Knowledge and truth',
    family: 'knowledge',
    purpose: 'Keep claims linked to evidence.',
    suitableProblemTypes: ['factual answer'],
    triggerConditions: ['verify'],
    requiredInputs: ['source'],
    producedOutputs: ['verified claim'],
    requiredAgents: ['evidence_agent'],
    workflowTemplate: ['retrieve', 'verify'],
    decisionRules: ['do not guess'],
    safetyInvariants: ['unsupported claims cannot become facts'],
    authorityRequirement: 'verify only',
    maximumAutonomyLevel: 4,
    riskCeiling: 'high',
    evidenceRequirements: ['source link'],
    evaluationMethod: ['claim support'],
    conflictsWith: [],
    userSpecificAdaptations: [],
    source: 'HAI specification',
    provenance: 'built in',
    status: 'active',
    effectiveStatus: 'active',
    enabled: true,
    pinned: false,
    effectiveAutonomyLevel: 4,
    adaptations: [],
  };
  const selection: IFrameworkSelectionDecision = {
    id: 'selection-1',
    taskPlanId: 'plan-1',
    createdAt: '2026-07-30T10:00:00Z',
    catalogVersion: 'v1',
    catalogDigest: 'a'.repeat(64),
    selectorAlgorithmVersion: 'selector-v3',
    effectivePreferenceDigest: 'b'.repeat(64),
    constitutionDigest: 'c'.repeat(64),
    lifeDomain: 'legal',
    needOrCommitment: 'respond to deadline',
    selected: [{
      id: framework.id,
      version: framework.version,
      name: framework.name,
      family: framework.family,
      score: 9,
      reasons: ['evidence is required'],
      maximumAutonomyLevel: 3,
      authorityRequirement: 'draft only',
      evidenceRequirements: ['source link'],
      evaluationMethod: ['claim support'],
    }],
    conflicts: [],
    requiredAgents: ['evidence_agent'],
    maximumAutonomyLevel: 2,
    authoritySummary: 'Draft only',
    requiresApproval: true,
    approvalReasons: ['legal communication'],
    evidenceRequirements: ['source link'],
    completionCriteria: ['draft is source grounded'],
    learningPlan: [],
    contextRequirements: ['case documents'],
    selectionReason: 'Legal work needs evidence and approval.',
    constitutionVersion: 1,
    constitutionSource: 'builtin-robert-constitution-v1:v1',
  };
  const constitution: IConstitution = {
    id: 'constitution-1',
    version: 1,
    baseVersion: 0,
    status: 'active',
    values: ['Operator sovereignty'],
    prohibitions: ['No self-authority'],
    standingPermissions: ['Read local sources'],
    preferences: ['Local first'],
    relationshipRules: [],
    financialBoundaries: ['No paid calls'],
    communicationRules: ['Draft legal messages'],
    escalationRules: ['Ask Robert'],
    protectedRules: ['Robert is final authority'],
    createdAt: '2026-07-30T10:00:00Z',
  };
  const overview: IFrameworkRegistryOverview = {
    generatedAt: '2026-07-30T10:00:00Z',
    total: 55,
    enabled: 50,
    experimental: 5,
    deprecated: 0,
    pinned: 1,
    families: { knowledge: 5 },
    constitutionVersion: 1,
    constitutionSource: 'builtin-robert-constitution-v1:v1',
    recentSelections: 1,
    selectionContract: ['classify the task'],
  };

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(FrameworkRegistryService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads the registry overview', () => {
    service.overview().subscribe((result) => expect(result).toEqual(overview));

    const request = http.expectOne('/api/v1/framework-registry/overview');
    expect(request.request.method).toBe('GET');
    request.flush(overview);
  });

  it('normalizes a wrapped framework list', () => {
    service.frameworks().subscribe((result) => expect(result).toEqual([framework]));

    const request = http.expectOne('/api/v1/framework-registry/frameworks');
    expect(request.request.method).toBe('GET');
    request.flush({ frameworks: [framework] });
  });

  it('loads one framework for inspection', () => {
    service.framework('truth-evidence').subscribe((result) => expect(result).toEqual(framework));

    const request = http.expectOne('/api/v1/framework-registry/frameworks/truth-evidence');
    expect(request.request.method).toBe('GET');
    request.flush({ framework });
  });

  it('posts only safe public selection intent fields', () => {
    const body = {
      request: 'Verify this claim',
      needsDocuments: true,
      executeRequested: true,
      requiredReasoning: 'high',
      riskLevel: 'low',
      needsApproval: false,
      humanApproved: true,
    } as IFrameworkSelectionRequest & {
      riskLevel: string;
      needsApproval: boolean;
      humanApproved: boolean;
    };
    service.select(body).subscribe((result) => expect(result).toEqual(selection));

    const request = http.expectOne('/api/v1/framework-registry/select');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      request: 'Verify this claim',
      needsDocuments: true,
      executeRequested: true,
      requiredReasoning: 'high',
    });
    expect(request.request.body['riskLevel']).toBeUndefined();
    expect(request.request.body['needsApproval']).toBeUndefined();
    expect(request.request.body['humanApproved']).toBeUndefined();
    request.flush({ selection });
  });

  it('patches an owner-scoped framework preference', () => {
    const body = {
      state: 'enabled' as const,
      pinned: true,
      maximumAutonomyLevel: 2,
      ownerIdentity: 'forged-owner',
    };
    service.updatePreference('truth-evidence', body).subscribe((result) => expect(result).toEqual(framework));

    const request = http.expectOne('/api/v1/framework-registry/frameworks/truth-evidence/preference');
    expect(request.request.method).toBe('PATCH');
    expect(request.request.body).toEqual({
      state: 'enabled',
      pinned: true,
      maximumAutonomyLevel: 2,
    });
    expect(request.request.body['ownerIdentity']).toBeUndefined();
    request.flush({ framework });
  });

  it('normalizes selection history', () => {
    service.selections().subscribe((result) => expect(result).toEqual([selection]));

    const request = http.expectOne('/api/v1/framework-registry/selections');
    expect(request.request.method).toBe('GET');
    request.flush({ selections: [selection] });
  });

  it('loads the active Constitution', () => {
    service.constitution().subscribe((result) => expect(result).toEqual({
      constitution,
      source: 'builtin-robert-constitution-v1:v1',
    }));

    const request = http.expectOne('/api/v1/framework-registry/constitution');
    expect(request.request.method).toBe('GET');
    request.flush({
      constitution,
      source: 'builtin-robert-constitution-v1:v1',
    });
  });

  it('loads and validates the owner Constitution version ledger', () => {
    const history: IConstitutionHistoryPage = {
      history: [{
        id: 'constitution-1',
        version: 1,
        baseVersion: 0,
        status: 'active',
        changeSummary: 'Initial owner governance baseline',
        approvedBy: 'owner-1',
        approvedAt: '2026-07-30T10:00:00Z',
        createdAt: '2026-07-30T09:00:00Z',
        digest: 'd'.repeat(64),
      }],
      limit: 50,
      truncated: false,
    };

    service.constitutionHistory().subscribe((result) => expect(result).toEqual(history));

    const request = http.expectOne(
      (candidate) =>
        candidate.url === '/api/v1/framework-registry/constitution/history' &&
        candidate.params.get('limit') === '50'
    );
    expect(request.request.method).toBe('GET');
    request.flush(history);
  });

  it('creates an inactive Constitution draft', () => {
    const body = {
      baseVersion: 1,
      values: ['Operator sovereignty'],
      prohibitions: [],
      standingPermissions: [],
      preferences: [],
      relationshipRules: [],
      financialBoundaries: [],
      communicationRules: [],
      escalationRules: [],
      changeSummary: 'Clarify sovereignty',
      ownerIdentity: 'forged-owner',
      protectedRules: ['disable approvals'],
    };
    service.createConstitutionDraft(body).subscribe((result) => expect(result).toEqual(constitution));

    const request = http.expectOne('/api/v1/framework-registry/constitution/drafts');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      baseVersion: 1,
      values: ['Operator sovereignty'],
      prohibitions: [],
      standingPermissions: [],
      preferences: [],
      relationshipRules: [],
      financialBoundaries: [],
      communicationRules: [],
      escalationRules: [],
      changeSummary: 'Clarify sovereignty',
    });
    expect(request.request.body['ownerIdentity']).toBeUndefined();
    expect(request.request.body['protectedRules']).toBeUndefined();
    request.flush({ draft: constitution });
  });

  it('activates a Constitution draft with explicit confirmation', () => {
    const body = {
      confirmation: 'ACTIVATE CONSTITUTION',
      approvalNote: 'Reviewed by Robert',
      approvedBy: 'forged-owner',
    };
    service.activateConstitution('constitution-1', body).subscribe((result) => expect(result).toEqual(constitution));

    const request = http.expectOne('/api/v1/framework-registry/constitution/constitution-1/activate');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      confirmation: 'ACTIVATE CONSTITUTION',
      approvalNote: 'Reviewed by Robert',
    });
    expect(request.request.body['approvedBy']).toBeUndefined();
    request.flush({ constitution });
  });

  it('redacts credential-like text from inbound registry records', () => {
    const unsafeFramework = {
      ...framework,
      source: 'https://runtime.local/catalog?access_token=raw-token',
      provenance: 'Authorization: Bearer raw-secret',
      purpose: 'Use api_key=private-value for testing',
      candidateImplementations: ['sk-super-secret-credential'],
      metadata: {
        clientSecret: 'plain-secret-without-a-label',
        token: 'raw-generic-token',
      },
    } as IFrameworkView;
    let result: IFrameworkView | undefined;

    service.framework('truth-evidence').subscribe((response) => (result = response));

    const request = http.expectOne('/api/v1/framework-registry/frameworks/truth-evidence');
    request.flush({ framework: unsafeFramework });

    expect(result?.source).toContain('access_token=[redacted]');
    expect(result?.provenance).toContain('Authorization: [redacted]');
    expect(result?.purpose).toContain('api_key=[redacted]');
    expect(JSON.stringify(result)).not.toContain('clientSecret');
    expect(JSON.stringify(result)).not.toContain('"token"');
    expect(JSON.stringify(result)).not.toContain('raw-secret');
    expect(JSON.stringify(result)).not.toContain('private-value');
    expect(JSON.stringify(result)).not.toContain('raw-token');
    expect(JSON.stringify(result)).not.toContain('plain-secret-without-a-label');
    expect(JSON.stringify(result)).not.toContain('raw-generic-token');
    expect(JSON.stringify(result)).not.toContain('sk-super-secret-credential');
  });

  it('rejects malformed list envelopes instead of presenting them as empty data', () => {
    let error: unknown;
    service.frameworks().subscribe({ error: (value) => (error = value) });

    const request = http.expectOne('/api/v1/framework-registry/frameworks');
    request.flush({ frameworks: { id: 'not-an-array' } });

    expect(error).toEqual(jasmine.any(Error));
    expect((error as Error).message).toBe(
      'Invalid Framework Registry framework list response.'
    );
  });

  it('rejects malformed selection provenance before it reaches the UI', () => {
    let error: unknown;
    service.select({ request: 'Plan safely' }).subscribe({
      error: (value) => (error = value),
    });

    const request = http.expectOne('/api/v1/framework-registry/select');
    request.flush({
      ...selection,
      constitutionDigest: 'not-a-sha256-digest',
    });

    expect(error).toEqual(jasmine.any(Error));
    expect((error as Error).message).toBe(
      'Invalid Framework Registry selection response.'
    );
  });

  it('rejects a malformed active Constitution timestamp', () => {
    let error: unknown;
    service.constitution().subscribe({ error: (value) => (error = value) });

    const request = http.expectOne('/api/v1/framework-registry/constitution');
    request.flush({
      constitution: { ...constitution, createdAt: 'not-a-date' },
      source: 'builtin-robert-constitution-v1:v1',
    });

    expect(error).toEqual(jasmine.any(Error));
    expect((error as Error).message).toBe(
      'Invalid Framework Registry active Constitution response.'
    );
  });
});
