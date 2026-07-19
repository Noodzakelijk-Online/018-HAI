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
    const catalogService = { adoptionPlan: jasmine.createSpy('adoptionPlan'), revalidate: jasmine.createSpy('revalidate'), revalidateOSSInsightCollections: jasmine.createSpy('revalidateOSSInsightCollections'), discoverOSSInsightRepositories: jasmine.createSpy('discoverOSSInsightRepositories'), discoverReviewableOSSInsightRepositories: jasmine.createSpy('discoverReviewableOSSInsightRepositories'), revalidateOSSInsightDiscovery: jasmine.createSpy('revalidateOSSInsightDiscovery'), recommendCapabilities: jasmine.createSpy('recommendCapabilities') }
    const pursuitService = { create: jasmine.createSpy('create') }
    const ragflowService = { status: jasmine.createSpy('status') }
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error'])
    const router = { navigate: jasmine.createSpy('navigate') }
    return {
      component: new BrainCatalogComponent(catalogService as any, pursuitService as any, ragflowService as any, notification, router as any),
      pursuitService,
      ragflowService,
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

  it('reads RAGFlow bridge state only when the RAGFlow candidate is selected', () => {
    const { component, ragflowService } = createComponent()
    ragflowService.status.and.returnValue(of({ enabled: false, configured: false, provider: 'RAGFlow', datasetCount: 0, capabilities: [], restrictions: ['no ingestion'], scope: 'candidate evidence only' }))

    component.select({ ...candidate, id: 'ragflow', name: 'RAGFlow' } as any)

    expect(ragflowService.status).toHaveBeenCalled()
    expect(component.ragflowStatus?.configured).toBeFalse()
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
