import { HttpErrorResponse } from '@angular/common/http'
import { of, throwError } from 'rxjs'
import { CapacitySnapshot, GoalNode, LifeDomain, NeedObservation } from '../../models/life-ops.model'
import { LifeOpsService } from '../../services/life-ops.service'
import { LifeOpsComponent } from './life-ops.component'

describe('LifeOpsComponent', () => {
  let service: jasmine.SpyObj<LifeOpsService>
  let notification: jasmine.SpyObj<any>
  let component: LifeOpsComponent

  const domain: LifeDomain = {
    id: 'health_wellbeing',
    name: 'Health and wellbeing',
    description: 'Physical and mental capacity.',
    needClass: 'physiological',
    sensitive: true,
  }
  const need = (id: string, overrides: Partial<NeedObservation> = {}): NeedObservation => ({
    id,
    ownerIdentity: 'owner-1',
    domainId: domain.id,
    needLevel: 'health',
    state: 'attention_required',
    currentLevel: 45,
    targetLevel: 75,
    gap: 30,
    priority: 70,
    confidence: 0.8,
    evidence: ['operator report'],
    sourceLabel: 'operator_report',
    observedAt: '2026-07-30T10:00:00Z',
    needsReview: false,
    createdAt: '2026-07-30T10:00:00Z',
    ...overrides,
  })
  const goal: GoalNode = {
    id: 'goal-1',
    ownerIdentity: 'owner-1',
    level: 'pursuit',
    domainIds: [domain.id],
    title: 'Restore daily capacity',
    successCriteria: ['Energy is stable'],
    stopConditions: ['Stop if symptoms worsen'],
    status: 'active',
    confidence: 0.8,
    sourceLabel: 'operator_goal',
    createdAt: '2026-07-30T10:00:00Z',
    updatedAt: '2026-07-30T10:00:00Z',
  }

  beforeEach(() => {
    service = jasmine.createSpyObj<LifeOpsService>('LifeOpsService', [
      'domains', 'needs', 'latestCapacity', 'goals', 'goalForest',
      'recordNeed', 'recordCapacity', 'linkEntity', 'entityDomains',
      'createGoal', 'updateGoal', 'assessPriority',
    ])
    notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error'])
    service.domains.and.returnValue(of([domain]))
    service.needs.and.returnValue(of([need('newest'), need('older')]))
    service.latestCapacity.and.returnValue(of(null))
    service.goals.and.returnValue(of([goal]))
    service.goalForest.and.returnValue(of([{ goal, children: [] }]))
    component = new LifeOpsComponent(service, notification)
  })

  it('loads a calm owner summary and keeps only the latest need per domain', () => {
    component.ngOnInit()

    expect(component.loading).toBeFalse()
    expect(component.errorMessage).toBe('')
    expect(component.currentNeeds.map((entry) => entry.id)).toEqual(['newest'])
    expect(component.capacityState).toBe('missing')
    expect(component.activeGoals).toEqual([goal])
    expect(component.goalForest.length).toBe(1)
  })

  it('shows review state for stale or explicitly reviewable context', () => {
    const capacity = {
      id: 'capacity-1',
      ownerIdentity: 'owner-1',
      status: 'available',
      signals: component.capacityForm.signals,
      timeAvailableMinutes: 60,
      concurrentWorkLimit: 1,
      currentLoad: 30,
      planningStepLimit: 4,
      constraints: [],
      sourceLabel: 'operator_report',
      capturedAt: '2026-07-28T10:00:00Z',
      confidence: 0.5,
      fresh: false,
      needsReview: true,
      createdAt: '2026-07-28T10:00:00Z',
    } as CapacitySnapshot
    service.latestCapacity.and.returnValue(of(capacity))
    service.needs.and.returnValue(of([need('review', { needsReview: true })]))

    component.refresh()

    expect(component.capacityState).toBe('review')
    expect(component.reviewNeeds.length).toBe(1)
  })

  it('records a need without inventing owner identity and updates the visible state', () => {
    component.ngOnInit()
    const saved = need('saved')
    service.recordNeed.and.returnValue(of(saved))
    component.needForm.domainId = domain.id

    component.saveNeed()

    expect(service.recordNeed).toHaveBeenCalled()
    expect((service.recordNeed.calls.mostRecent().args[0] as any).ownerIdentity).toBeUndefined()
    expect(component.needs[0].id).toBe('saved')
    expect(notification.success).toHaveBeenCalled()
  })

  it('preserves a useful API failure state instead of showing empty data', () => {
    service.domains.and.returnValue(throwError(() => new HttpErrorResponse({
      status: 503,
      error: { error: 'whole-life context operation failed' },
    })))

    component.refresh()

    expect(component.loading).toBeFalse()
    expect(component.errorMessage).toBe('whole-life context operation failed')
  })
})
