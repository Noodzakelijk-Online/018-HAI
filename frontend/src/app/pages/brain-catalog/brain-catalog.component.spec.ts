import { of, throwError } from 'rxjs'
import { BrainCatalogComponent } from './brain-catalog.component'

describe('BrainCatalogComponent adapter reviews', () => {
  const candidate = {
    id: 'cline',
    name: 'Cline',
    upstreamUrl: 'https://github.com/cline/cline',
    sourceCatalogUrl: 'https://ossinsight.io/collections/llm-devtools',
    status: 'candidate',
    category: 'interactive coding agent',
    integrationMode: 'local bridge',
    capabilities: [],
    recommendedFor: [],
    requiresApproval: true,
    localFirstCompatible: true,
    activation: 'review first',
    rationale: 'Tool-mediated workspace access needs a boundary.',
    verifiedAt: '2026-07-19',
    verificationNote: 'reviewed',
  }

  function createComponent() {
    const catalogService = { adoptionPlan: jasmine.createSpy('adoptionPlan'), revalidate: jasmine.createSpy('revalidate'), revalidationHistory: jasmine.createSpy('revalidationHistory'), runDueRevalidations: jasmine.createSpy('runDueRevalidations'), collectionRevalidationHistory: jasmine.createSpy('collectionRevalidationHistory'), repositoryDiscoveryRevalidationHistory: jasmine.createSpy('repositoryDiscoveryRevalidationHistory'), revalidateOSSInsightCollections: jasmine.createSpy('revalidateOSSInsightCollections'), discoverOSSInsightRepositories: jasmine.createSpy('discoverOSSInsightRepositories'), discoverReviewableOSSInsightRepositories: jasmine.createSpy('discoverReviewableOSSInsightRepositories'), revalidateOSSInsightDiscovery: jasmine.createSpy('revalidateOSSInsightDiscovery'), recommendCapabilities: jasmine.createSpy('recommendCapabilities') }
    const pursuitService = { create: jasmine.createSpy('create') }
    const ragflowService = { status: jasmine.createSpy('status') }
    const anythingLLMService = { status: jasmine.createSpy('status') }
    const presidioService = { status: jasmine.createSpy('status') }
		const langfuseService = { status: jasmine.createSpy('status'), probe: jasmine.createSpy('probe'), exportOperationalSnapshot: jasmine.createSpy('exportOperationalSnapshot') }
    const openLITService = { status: jasmine.createSpy('status'), exportOperationalSnapshot: jasmine.createSpy('exportOperationalSnapshot') }
    const serenaService = { status: jasmine.createSpy('status') }
    const mlflowService = { status: jasmine.createSpy('status') }
		const miniSWEService = { status: jasmine.createSpy('status') }
		const autoGenCompatibilityService = { microsoftAgentFrameworkMigrationPlan: jasmine.createSpy('microsoftAgentFrameworkMigrationPlan') }
    const gitleaksService = { status: jasmine.createSpy('status') }
		const gosecService = { status: jasmine.createSpy('status') }
		const trivyService = { status: jasmine.createSpy('status') }
    const grypeService = { status: jasmine.createSpy('status') }
    const syftService = { status: jasmine.createSpy('status') }
    const browserVerificationService = { status: jasmine.createSpy('status') }
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error', 'info'])
    const router = { navigate: jasmine.createSpy('navigate') }
    return {
		component: new BrainCatalogComponent(catalogService as any, pursuitService as any, ragflowService as any, anythingLLMService as any, presidioService as any, langfuseService as any, openLITService as any, serenaService as any, mlflowService as any, miniSWEService as any, autoGenCompatibilityService as any, gitleaksService as any, gosecService as any, trivyService as any, grypeService as any, syftService as any, browserVerificationService as any, notification, router as any),
      pursuitService,
      ragflowService,
      anythingLLMService,
      presidioService,
			langfuseService,
      openLITService,
      serenaService,
      mlflowService,
			miniSWEService,
			autoGenCompatibilityService,
      gitleaksService,
			gosecService,
			trivyService,
			grypeService,
      syftService,
      browserVerificationService,
      notification,
      router,
    }
  }

  it('creates a review pursuit without claiming activation', () => {
    const { component, pursuitService, notification, router } = createComponent()
    pursuitService.create.and.returnValue(of({ id: 'pursuit-1' }))

    component.startAdapterReview(candidate as any)

    expect(pursuitService.create).toHaveBeenCalledWith(jasmine.objectContaining({
      title: 'Review Cline adapter boundary',
      status: 'waiting',
      autonomyLevel: 'manual',
      riskLevel: 'high',
      sourceOfCreation: 'brain_catalog:cline',
    }))
    expect(notification.success).toHaveBeenCalledWith('Adapter review created', 'Cline remains disabled. HAI created a review record instead of activating the project.')
    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: { selected: 'pursuit-1' } })
  })

  it('checks Grype status only when its bounded catalog profile is selected', () => {
    const { component, grypeService } = createComponent()
    grypeService.status.and.returnValue(of({ configured: false, workspaces: [], restrictions: [], scope: 'aggregate only' }))

    component.select({ ...candidate, id: 'grype', name: 'Grype', status: 'integrated_profile' } as any)

    expect(grypeService.status).toHaveBeenCalled()
    expect(component.grypeStatus?.configured).toBeFalse()
  })

		it('checks Gosec status only when its bounded catalog profile is selected', () => {
			const { component, gosecService } = createComponent()
			gosecService.status.and.returnValue(of({ configured: false, workspaces: [], restrictions: [], scope: 'aggregate only' }))

			component.select({ ...candidate, id: 'gosec', name: 'Gosec', status: 'integrated_profile' } as any)

			expect(gosecService.status).toHaveBeenCalled()
			expect(component.gosecStatus?.configured).toBeFalse()
		})

		it('checks Trivy status only when its bounded catalog profile is selected', () => {
			const { component, trivyService } = createComponent()
			trivyService.status.and.returnValue(of({ configured: false, workspaces: [], restrictions: [], scope: 'aggregate only' }))

			component.select({ ...candidate, id: 'trivy', name: 'Trivy', status: 'integrated_profile' } as any)

			expect(trivyService.status).toHaveBeenCalled()
			expect(component.trivyStatus?.configured).toBeFalse()
		})

  it('does not create a review for held catalog entries', () => {
    const { component, pursuitService } = createComponent()
    component.startAdapterReview({ ...candidate, status: 'excluded' } as any)
    expect(pursuitService.create).not.toHaveBeenCalled()
  })

  it('loads the read-only implementation roadmap without activating a project', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.adoptionPlan.and.returnValue(of({ items: [{ id: 'cloudquery', name: 'CloudQuery', priority: 88 }], message: 'does not install or execute' }))

    component.loadAdoptionPlan()

    expect(catalogService.adoptionPlan).toHaveBeenCalled()
    expect(component.adoptionPlan?.items[0].id).toBe('cloudquery')
    expect(notification.error).not.toHaveBeenCalled()
  })

  it('routes a roadmap candidate through the existing manual adapter review flow', () => {
    const { component, pursuitService } = createComponent()
    component.catalog = { entries: [candidate] } as any
    pursuitService.create.and.returnValue(of({ id: 'pursuit-roadmap-1' }))

    component.startRoadmapReview('cline')

    expect(pursuitService.create).toHaveBeenCalledWith(jasmine.objectContaining({ sourceOfCreation: 'brain_catalog:cline' }))
  })

  it('keeps the candidate disabled after a create failure', () => {
    const { component, pursuitService, notification } = createComponent()
    pursuitService.create.and.returnValue(throwError(() => new Error('offline')))

    component.startAdapterReview(candidate as any)

    expect(component.reviewingCandidateId).toBe('')
    expect(notification.error).toHaveBeenCalledWith('Could not create adapter review', 'No project was installed, configured, or activated. Try again after checking the local pursuit service.')
  })

  it('shows an upstream recheck without changing the catalog entry', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.revalidate.and.returnValue(of({
      id: 'cline',
      available: true,
      archived: false,
      license: 'Apache-2.0',
      message: 'metadata only',
    }))

    component.revalidate(candidate as any)

    expect(catalogService.revalidate).toHaveBeenCalledWith('cline')
    expect(component.upstreamReview?.license).toBe('Apache-2.0')
    expect(notification.success).toHaveBeenCalledWith('Upstream rechecked', 'Cline was checked without changing its HAI activation state.')
  })

  it('runs the bounded catalog maintenance sweep without claiming activation', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.collectionRevalidationHistory.and.returnValue(of([]))
		catalogService.repositoryDiscoveryRevalidationHistory.and.returnValue(of([]))
    catalogService.runDueRevalidations.and.returnValue(of({ enabled: true, eligible: 4, checked: 2, reused: 2, failed: 0, results: [], runAt: '2026-07-21T13:00:00Z' }))

    component.runDueCatalogRevalidations()

    expect(catalogService.runDueRevalidations).toHaveBeenCalled()
    expect(component.catalogRevalidation?.checked).toBe(2)
    expect(notification.success).toHaveBeenCalledWith('Catalog maintenance complete', '2 checked, 2 still current, 0 unavailable or failed.')
  })

  it('shows durable collection-index evidence without starting a replacement scan', () => {
    const { component } = createComponent()
    const catalogService = (component as any).service
    catalogService.collectionRevalidationHistory.and.returnValue(of([{ checkedAt: '2026-07-21T13:00:00Z', available: true, expectedTotal: 138, currentTotal: 138, message: 'source snapshot matches' }]))

    ;(component as any).loadCollectionMaintenanceHistory()

    expect(catalogService.collectionRevalidationHistory).toHaveBeenCalledWith()
    expect(component.collectionMaintenanceHistory.length).toBe(1)
    expect(component.collectionMaintenanceHistoryUnavailable).toBeFalse()
  })

	it('shows persisted repository gap-review evidence without starting a replacement scan', () => {
		const { component } = createComponent()
		const catalogService = (component as any).service
		catalogService.repositoryDiscoveryRevalidationHistory.and.returnValue(of([{ checkedAt: '2026-07-21T13:00:00Z', available: true, scope: 'reviewable', repositoriesChecked: 116, unreviewedDiscoveries: 2, candidatesTruncated: false, message: 'review only' }]))

		;(component as any).loadRepositoryDiscoveryMaintenanceHistory()

		expect(catalogService.repositoryDiscoveryRevalidationHistory).toHaveBeenCalledWith()
		expect(component.repositoryDiscoveryMaintenanceHistory.length).toBe(1)
		expect(component.repositoryDiscoveryMaintenanceHistoryUnavailable).toBeFalse()
	})

	it('reports collection drift without changing catalog or runtime state', () => {
	  const { component, notification } = createComponent()
	  const catalogService = (component as any).service
	  catalogService.revalidateOSSInsightCollections.and.returnValue(of({
	    available: true,
	    expectedTotal: 138,
	    currentTotal: 139,
	    newCollections: ['Future capability'],
	    missingExpected: [],
	    message: 'drift only',
	  }))

	  component.revalidateOSSInsightCollections()

	  expect(catalogService.revalidateOSSInsightCollections).toHaveBeenCalled()
	  expect(component.ossInsightReview?.newCollections).toEqual(['Future capability'])
	  expect(notification.success).toHaveBeenCalledWith('OSS Insight checked', 'Collection drift needs catalog review. No project was installed or activated.')
	})

  it('creates a manual discovery review without adding a catalog profile', () => {
    const { component, pursuitService, notification, router } = createComponent()
    pursuitService.create.and.returnValue(of({ id: 'pursuit-discovery-1' }))
    component.discoveryReviews['owner/new-mcp'] = { id: 'ossinsight-owner-new-mcp', name: 'owner/new-mcp', upstreamUrl: 'https://github.com/owner/new-mcp', repositoryMoved: false, checkedAt: '2026-07-21T00:00:00Z', available: true, archived: false, license: 'MIT', message: 'metadata only', disposition: 'candidate', readiness: 'review_now', readinessReason: 'review safely' }

    component.queueDiscoveryReview({ collection: 'MCP Servers', disposition: 'review_candidate', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'controlled execution', priority: 72, risk: 'high', reviewReason: 'Review locally.' })

    expect(pursuitService.create).toHaveBeenCalledWith(jasmine.objectContaining({
      title: 'Screen owner/new-mcp for a HAI adapter',
      status: 'waiting',
      autonomyLevel: 'manual',
      riskLevel: 'high',
      sourceOfCreation: 'ossinsight_discovery:owner/new-mcp',
    }))
    expect(notification.success).toHaveBeenCalledWith('Discovery review created', 'owner/new-mcp remains unconfigured. HAI created a manual review record.')
    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: { selected: 'pursuit-discovery-1' } })
  })

  it('does not create a discovery review before current metadata is verified', () => {
    const { component, pursuitService, notification } = createComponent()

    component.queueDiscoveryReview({ collection: 'MCP Servers', disposition: 'review_candidate', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'controlled execution', priority: 72, risk: 'high', reviewReason: 'Review locally.' })

    expect(pursuitService.create).not.toHaveBeenCalled()
    expect(notification.error).toHaveBeenCalledWith('Verify metadata first', jasmine.stringContaining('No repository was added to the catalog or activated.'))
  })

  it('does not create a discovery review for an unavailable or archived upstream', () => {
    const { component, pursuitService, notification } = createComponent()
    component.discoveryReviews['owner/new-mcp'] = { id: 'ossinsight-owner-new-mcp', name: 'owner/new-mcp', upstreamUrl: 'https://github.com/owner/new-mcp', repositoryMoved: false, checkedAt: '2026-07-21T00:00:00Z', available: false, archived: true, message: 'not found', disposition: 'candidate', readiness: 'unavailable', readinessReason: 'repository is unavailable' }

    component.queueDiscoveryReview({ collection: 'MCP Servers', disposition: 'review_candidate', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'controlled execution', priority: 72, risk: 'high', reviewReason: 'Review locally.' })

    expect(pursuitService.create).not.toHaveBeenCalled()
    expect(notification.error).toHaveBeenCalledWith('Upstream is unavailable', jasmine.stringContaining('unavailable or archived'))
  })

  it('does not create a discovery review for a source-recorded reference or deferred capability', () => {
    const { component, pursuitService, notification } = createComponent()

    component.queueDiscoveryReview({ collection: 'RAG Frameworks', disposition: 'review_candidate', repository: 'HKUDS/RAG-Anything', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'source intake', priority: 76, risk: 'high', reviewReason: 'Review locally.', triage: 'reference_only', triageReason: 'Overlaps the existing RAGFlow bridge.', reviewAllowed: false })

    expect(pursuitService.create).not.toHaveBeenCalled()
    expect(notification.info).toHaveBeenCalledWith('Discovery is not an adapter candidate', 'Overlaps the existing RAGFlow bridge.')
  })

  it('verifies source-discovered repository metadata before a manual review', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.revalidateOSSInsightDiscovery.and.returnValue(of({ id: 'ossinsight-owner-new-mcp', name: 'owner/new-mcp', upstreamUrl: 'https://github.com/owner/new-mcp', available: true, archived: false, license: 'MIT', message: 'metadata only', disposition: 'candidate', readiness: 'review_now', readinessReason: 'review safely' }))

    component.verifyDiscovery({ collection: 'MCP Servers', disposition: 'review_candidate', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'controlled execution', priority: 72, risk: 'high', reviewReason: 'Review locally.' })

    expect(catalogService.revalidateOSSInsightDiscovery).toHaveBeenCalledWith('owner/new-mcp', 'candidate')
    expect(component.discoveryReviews['owner/new-mcp'].license).toBe('MIT')
    expect(notification.success).toHaveBeenCalled()
  })

  it('matches a need against the reviewed catalog without activating a candidate', () => {
    const { component } = createComponent()
    const catalogService = (component as any).service
    catalogService.recommendCapabilities.and.returnValue(of({ need: 'local model evaluation', message: 'planning only', recommendations: [{ id: 'lm-eval-harness', name: 'LM Evaluation Harness', status: 'candidate', role: 'offline model evaluation', rationale: 'test', requiresApproval: true, activation: 'review first', score: 14, reasons: ['matches capability'], nextStep: 'Create a manual adapter review.' }] }))

    component.recommendCapabilities('local model evaluation')

    expect(catalogService.recommendCapabilities).toHaveBeenCalledWith('local model evaluation')
    expect(component.capabilityRecommendation?.recommendations[0].id).toBe('lm-eval-harness')
  })

  it('opens Runtime Lab for the MCP Toolbox inspection profile', () => {
    const { component, router } = createComponent()

    component.openMCPToolboxRuntimeLab()

    expect(router.navigate).toHaveBeenCalledWith(['/runtime-lab'])
  })

  it('reads RAGFlow bridge state only when the RAGFlow candidate is selected', () => {
    const { component, ragflowService } = createComponent()
    ragflowService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'RAGFlow', datasetCount: 0, capabilities: [], restrictions: ['no ingestion'], scope: 'candidate evidence only' }))

    component.select({ ...candidate, id: 'ragflow', name: 'RAGFlow' } as any)

    expect(ragflowService.status).toHaveBeenCalled()
    expect(component.ragflowStatus?.configured).toBeFalse()
  })

  it('reads AnythingLLM bridge state only when the AnythingLLM profile is selected', () => {
    const { component, anythingLLMService } = createComponent()
    anythingLLMService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'AnythingLLM', workspaceCount: 0, workspaceSlugs: [], localEmbeddingsConfirmed: false, capabilities: [], restrictions: ['no chat'], scope: 'candidate evidence only' }))

    component.select({ ...candidate, id: 'anythingllm', name: 'AnythingLLM', status: 'integrated_profile' } as any)

    expect(anythingLLMService.status).toHaveBeenCalled()
    expect(component.anythingLLMStatus?.configured).toBeFalse()
  })

  it('reads Presidio bridge state only when the Presidio candidate is selected', () => {
    const { component, presidioService } = createComponent()
    presidioService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'Presidio Analyzer', language: '', entityTypes: [], capabilities: [], restrictions: ['no persistence'], scope: 'review metadata only' }))

    component.select({ ...candidate, id: 'presidio', name: 'Presidio' } as any)

    expect(presidioService.status).toHaveBeenCalled()
    expect(component.presidioStatus?.configured).toBeFalse()
  })

  it('reads Langfuse bridge state only when the Langfuse profile is selected', () => {
    const { component, langfuseService } = createComponent()
    langfuseService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'Langfuse self-hosted observability', capabilities: [], restrictions: ['no prompt export'], scope: 'aggregate-only local trace evidence' }))

    component.select({ ...candidate, id: 'langfuse', name: 'Langfuse', status: 'integrated_profile' } as any)

    expect(langfuseService.status).toHaveBeenCalled()
    expect(component.langfuseStatus?.configured).toBeFalse()
  })

  it('reads OpenLIT bridge state only when the OpenLIT profile is selected', () => {
    const { component, openLITService } = createComponent()
    openLITService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'OpenLIT local OTLP observability', capabilities: [], restrictions: ['no prompt export'], scope: 'aggregate-only local trace evidence' }))

    component.select({ ...candidate, id: 'openlit', name: 'OpenLIT', status: 'integrated_profile' } as any)

    expect(openLITService.status).toHaveBeenCalled()
    expect(component.openLITStatus?.configured).toBeFalse()
  })

  it('reads Serena bridge state only when the Serena profile is selected', () => {
    const { component, serenaService } = createComponent()
    serenaService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'Serena semantic code context', capabilities: [], restrictions: ['no edit'], scope: 'read-only metadata only' }))

    component.select({ ...candidate, id: 'serena', name: 'Serena', status: 'integrated_profile' } as any)

    expect(serenaService.status).toHaveBeenCalled()
    expect(component.serenaStatus?.configured).toBeFalse()
  })

  it('reads MLflow evidence state only when the MLflow profile is selected', () => {
    const { component, mlflowService } = createComponent()
    mlflowService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'MLflow local evaluation evidence', experimentIds: [], metricKeys: [], capabilities: [], restrictions: ['no prompts'], scope: 'manual review context only' }))

    component.select({ ...candidate, id: 'mlflow', name: 'MLflow', status: 'integrated_profile' } as any)

    expect(mlflowService.status).toHaveBeenCalled()
    expect(component.mlflowStatus?.configured).toBeFalse()
  })

  it('reads mini-SWE state only when the disposable patch worker profile is selected', () => {
    const { component, miniSWEService } = createComponent()
    miniSWEService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'mini-SWE-agent disposable patch proposal runner', workspaces: [], capabilities: [], restrictions: ['no apply'], scope: 'review-only patch proposals' }))

    component.select({ ...candidate, id: 'mini-swe-agent', name: 'mini-SWE-agent', status: 'integrated_profile' } as any)

    expect(miniSWEService.status).toHaveBeenCalled()
    expect(component.miniSWEStatus?.configured).toBeFalse()
  })

  it('reads Gitleaks state only when the aggregate secret-scan profile is selected', () => {
    const { component, gitleaksService } = createComponent()
    gitleaksService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'Gitleaks local aggregate secret scanner', workspaces: [], capabilities: [], restrictions: ['no source content'], scope: 'aggregate-only safety evidence' }))

    component.select({ ...candidate, id: 'gitleaks', name: 'Gitleaks', status: 'integrated_profile' } as any)

    expect(gitleaksService.status).toHaveBeenCalled()
    expect(component.gitleaksStatus?.configured).toBeFalse()
  })

  it('reads Syft state only when the aggregate SBOM profile is selected', () => {
    const { component, syftService } = createComponent()
    syftService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'Syft local aggregate SBOM inventory', workspaces: [], capabilities: [], restrictions: ['no package details'], scope: 'aggregate-only supply-chain evidence' }))

    component.select({ ...candidate, id: 'syft', name: 'Syft', status: 'integrated_profile' } as any)

    expect(syftService.status).toHaveBeenCalled()
    expect(component.syftStatus?.configured).toBeFalse()
  })

  it('reads browser verification state only when the Playwright profile is selected', () => {
    const { component, browserVerificationService } = createComponent()
    browserVerificationService.status.and.returnValue(of({ enabled: false, configured: false, profiles: [], scope: 'named read-only local checks only' }))

    component.select({ ...candidate, id: 'playwright', name: 'Playwright', status: 'integrated_profile' } as any)

    expect(browserVerificationService.status).toHaveBeenCalled()
    expect(component.browserVerificationStatus?.configured).toBeFalse()
  })

  it('probes Langfuse without exporting a trace', () => {
    const { component, langfuseService, notification } = createComponent()
    langfuseService.probe.and.returnValue(of({ healthy: true, ready: true, checkedAt: '2026-07-20T00:00:00Z', scope: 'health only' }))

    component.probeLangfuse()

    expect(langfuseService.probe).toHaveBeenCalled()
    expect(component.langfuseProbe?.ready).toBeTrue()
    expect(notification.success).toHaveBeenCalledWith('Langfuse ready', 'HAI verified the configured local health and readiness endpoints. No trace was exported.')
  })

  it('exports only through the explicit Langfuse snapshot action', () => {
    const { component, langfuseService, notification } = createComponent()
    langfuseService.exportOperationalSnapshot.and.returnValue(of({ traceId: 'a'.repeat(32), spanId: 'b'.repeat(16), exportedAt: '2026-07-20T00:00:00Z', scope: 'aggregate-only' }))

    component.exportLangfuseSnapshot()

    expect(langfuseService.exportOperationalSnapshot).toHaveBeenCalled()
    expect(component.langfuseExport?.traceId).toBe('a'.repeat(32))
    expect(notification.success).toHaveBeenCalledWith('Aggregate trace exported', "Langfuse accepted HAI's fixed aggregate operational snapshot. No prompts, sources, or workflow records were exported.")
  })

  it('exports only through the explicit OpenLIT snapshot action', () => {
    const { component, openLITService, notification } = createComponent()
    openLITService.exportOperationalSnapshot.and.returnValue(of({ traceId: 'a'.repeat(32), spanId: 'b'.repeat(16), exportedAt: '2026-07-21T00:00:00Z', scope: 'aggregate-only' }))

    component.exportOpenLITSnapshot()

    expect(openLITService.exportOperationalSnapshot).toHaveBeenCalled()
    expect(component.openLITExport?.traceId).toBe('a'.repeat(32))
    expect(notification.success).toHaveBeenCalledWith('Aggregate trace exported', "OpenLIT accepted HAI's fixed aggregate operational snapshot. No prompts, sources, tokens, models, or workflow records were exported.")
  })

  it('prepares a transient Microsoft Agent Framework migration plan without activation', () => {
    const { component, autoGenCompatibilityService, notification } = createComponent()
    autoGenCompatibilityService.microsoftAgentFrameworkMigrationPlan.and.returnValue(of({
      target: 'microsoft-agent-framework',
      preview: { workloadId: 'legacy', openLoops: [], recommendedControls: [], requiresReview: true, executionAllowed: false, persistenceAllowed: false },
      steps: [{ order: 1, haiControl: 'task intake', agentFrameworkRole: 'map ingress', gate: 'review first' }],
      blockedUntil: ['owner review'],
      executionAllowed: false,
      frameworkRuntimeDetected: false,
      scope: 'did not install framework',
    }))

    component.prepareFrameworkMigration(JSON.stringify({ workloadId: 'legacy', events: [{ id: 'e1', type: 'message', summary: 'Review' }] }))

    expect(autoGenCompatibilityService.microsoftAgentFrameworkMigrationPlan).toHaveBeenCalledWith('legacy', [{ id: 'e1', type: 'message', summary: 'Review' }])
    expect(component.frameworkMigrationPlan?.frameworkRuntimeDetected).toBeFalse()
    expect(notification.success).toHaveBeenCalledWith('Migration plan prepared', 'HAI created a transient control map. Microsoft Agent Framework was not installed or started.')
  })

  it('rejects an invalid migration sample locally without a request', () => {
    const { component, autoGenCompatibilityService, notification } = createComponent()
    component.prepareFrameworkMigration('{')
    expect(autoGenCompatibilityService.microsoftAgentFrameworkMigrationPlan).not.toHaveBeenCalled()
    expect(notification.error).toHaveBeenCalledWith('Invalid migration sample', 'Use a redacted JSON object with workloadId and 1-100 supported event envelopes. Nothing was sent to an agent runtime.')
  })

  it('shows discovery results without changing runtime state', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.discoverOSSInsightRepositories.and.returnValue(of({
      available: true,
      cached: false,
      collectionsScreened: 138,
      candidateCollections: 12,
      collectionsChecked: 12,
      repositoriesChecked: 50,
      duplicateSourceHits: 0,
      maximumDiscoveries: 800,
      knownProfileHits: 8,
      discoveries: [{ collection: 'MCP Servers', disposition: 'review_candidate', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'controlled execution', priority: 72, risk: 'high', reviewReason: 'Review locally.', relatedCollections: ['MCP Servers'], relatedSourceUrls: ['https://api.ossinsight.io/example'] }],
      discoveriesTruncated: false,
      message: 'did not add catalog entries',
    }))

    component.discoverOSSInsightRepositories()

    expect(catalogService.discoverOSSInsightRepositories).toHaveBeenCalled()
    expect(component.ossInsightDiscovery?.discoveries?.[0].repository).toBe('owner/new-mcp')
    expect(notification.success).toHaveBeenCalledWith('Candidate discovery complete', '1 unreviewed repositories were found. No catalog entry, credential, or runtime state changed.')
  })

  it('keeps already-profiled source repositories separate from new review candidates', () => {
    const { component } = createComponent()
    component.ossInsightDiscovery = {
      knownProfiles: Array.from({ length: 21 }, (_, index) => ({ repository: `owner/profile-${index}`, catalogEntryIds: ['ollama'], relatedCollections: ['LLM Inference Engines'] })),
    } as any

    expect(component.visibleKnownProfiles.length).toBe(20)
    expect(component.visibleKnownProfiles[0].catalogEntryIds).toEqual(['ollama'])

    component.showMoreKnownProfiles()

    expect(component.visibleKnownProfiles.length).toBe(21)
  })

  it('can scan represented capability categories without activating an upstream', () => {
    const { component, notification } = createComponent()
    const catalogService = (component as any).service
    catalogService.discoverReviewableOSSInsightRepositories.and.returnValue(of({
      available: true,
      cached: false,
      scope: 'reviewable',
      collectionsScreened: 138,
      candidateCollections: 12,
      reviewableCollections: 25,
      eligibleCollections: 25,
      collectionsChecked: 25,
      repositoriesChecked: 90,
      duplicateSourceHits: 0,
      maximumDiscoveries: 800,
      knownProfileHits: 8,
      discoveries: [{ collection: 'LLM Inference Engines', disposition: 'represented_in_catalog', repository: 'owner/new-inference', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.', reviewTrack: 'local inference', priority: 80, risk: 'medium', reviewReason: 'Review loopback limits.', relatedCollections: ['LLM Inference Engines'], relatedSourceUrls: ['https://api.ossinsight.io/example'] }],
      discoveriesTruncated: false,
      message: 'did not add catalog entries',
    }))

    component.discoverReviewableOSSInsightRepositories()

    expect(catalogService.discoverReviewableOSSInsightRepositories).toHaveBeenCalled()
    expect(component.ossInsightDiscovery?.scope).toBe('reviewable')
    expect(notification.success).toHaveBeenCalledWith('Relevant discovery complete', '1 unreviewed repositories were found across candidate and represented categories. No catalog entry, credential, or runtime state changed.')
  })
})
