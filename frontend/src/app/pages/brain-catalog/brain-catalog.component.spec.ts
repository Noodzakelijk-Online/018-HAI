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
    const catalogService = {} as any
    const pursuitService = { create: jasmine.createSpy('create') }
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error'])
    const router = { navigate: jasmine.createSpy('navigate') }
    return {
      component: new BrainCatalogComponent(catalogService, pursuitService as any, notification, router as any),
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
})
