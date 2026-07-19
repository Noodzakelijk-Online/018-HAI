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
    const catalogService = { revalidate: jasmine.createSpy('revalidate'), revalidateOSSInsightCollections: jasmine.createSpy('revalidateOSSInsightCollections'), discoverOSSInsightRepositories: jasmine.createSpy('discoverOSSInsightRepositories') }
    const pursuitService = { create: jasmine.createSpy('create') }
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error'])
    const router = { navigate: jasmine.createSpy('navigate') }
    return {
      component: new BrainCatalogComponent(catalogService as any, pursuitService as any, notification, router as any),
      pursuitService,
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

    component.queueDiscoveryReview({ collection: 'MCP Servers', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.' })

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
      knownProfileHits: 8,
      discoveries: [{ collection: 'MCP Servers', repository: 'owner/new-mcp', sourceUrl: 'https://api.ossinsight.io/example', rationale: 'Review first.' }],
      discoveriesTruncated: false,
      message: 'did not add catalog entries',
    }))

    component.discoverOSSInsightRepositories()

    expect(catalogService.discoverOSSInsightRepositories).toHaveBeenCalled()
    expect(component.ossInsightDiscovery?.discoveries?.[0].repository).toBe('owner/new-mcp')
    expect(notification.success).toHaveBeenCalledWith('Candidate discovery complete', '1 unreviewed repositories were found. No catalog entry, credential, or runtime state changed.')
  })
})
