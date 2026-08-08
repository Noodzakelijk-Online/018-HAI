import { HttpErrorResponse } from '@angular/common/http'
import { CommonModule } from '@angular/common'
import { ComponentFixture, TestBed } from '@angular/core/testing'
import { FormsModule } from '@angular/forms'
import { BrowserAnimationsModule } from '@angular/platform-browser/animations'
import { ActivatedRoute } from '@angular/router'
import {
  AuditOutline,
  BranchesOutline,
  CheckCircleOutline,
  CloseOutline,
  DownOutline,
  LinkOutline,
  NodeIndexOutline,
  PauseCircleOutline,
  ProfileOutline,
  ReloadOutline,
  RightOutline,
  SafetyCertificateOutline,
  UpOutline,
  WarningOutline,
} from '@ant-design/icons-angular/icons'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzDrawerModule } from 'ng-zorro-antd/drawer'
import { NZ_ICONS, NzIconModule } from 'ng-zorro-antd/icon'
import { NzNotificationModule, NzNotificationService } from 'ng-zorro-antd/notification'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { of, throwError } from 'rxjs'
import { ControlRoomModule } from '../../control-room/control-room.module'
import { IPlanGraph, IPlanNode } from '../../models/plan-graph.model.interface'
import { PlanGraphService } from '../../services/plan-graph.service'
import { PlanCoordinationComponent } from './plan-coordination.component'

describe('PlanCoordinationComponent', () => {
  let service: jasmine.SpyObj<PlanGraphService>
  let notification: jasmine.SpyObj<any>
  let component: PlanCoordinationComponent

  const node = (overrides: Partial<IPlanNode> = {}): IPlanNode => ({
    id: 'node-1',
    sequence: 1,
    title: 'Collect evidence',
    status: 'ready',
    ownerType: 'hai',
    risk: 'low',
    approvalRequired: false,
    approvalStatus: 'not_required',
    dependencyIds: [],
    constraints: [],
    resourceEstimates: [],
    bindings: [],
    transport: {
      id: 'node-1',
      type: 'objective',
      title: 'Collect evidence',
      owner: 'hai',
      status: 'ready',
      estimatedMinutes: 0,
      estimatedCostEur: 0,
      risk: 'low',
      approvalState: 'not_required',
      bindings: {},
    },
    ...overrides,
  })

  const plan = (overrides: Partial<IPlanGraph> = {}): IPlanGraph => ({
    id: 'plan-1',
    title: 'Evidence response',
    objective: 'Prepare a verified response',
    status: 'draft',
    risk: 'medium',
    revision: 1,
    planDigest: 'digest-1',
    successCriteria: ['Sources attached'],
    nodes: [node()],
    edges: [],
    criticalPathNodeIds: ['node-1'],
    constraints: [],
    resourceEstimates: [],
    bindings: [],
    approval: { required: true, status: 'pending', reason: 'External communication' },
    createdAt: '2026-08-05T08:00:00Z',
    updatedAt: '2026-08-05T08:00:00Z',
    revisions: [],
    repairHistory: [],
    canExecute: false,
    ...overrides,
  })

  beforeEach(() => {
    service = jasmine.createSpyObj<PlanGraphService>('PlanGraphService', ['list', 'get', 'preview', 'accept', 'replan'])
    notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error'])
    service.list.and.returnValue(of([plan()]))
    service.get.and.callFake(() => of(plan()))
    component = new PlanCoordinationComponent(service, notification, {
      snapshot: { queryParamMap: { get: () => null } },
    } as any)
  })

  it('loads the immutable plan list and exposes the next coordinated node', () => {
    component.ngOnInit()

    expect(component.loading).toBeFalse()
    expect(component.selectedPlan?.id).toBe('plan-1')
    expect(component.nextCoordinatedNode?.id).toBe('node-1')
    expect(component.planRequiresDecision).toBeTrue()
  })

  it('selects and opens a plan requested by a workflow deep link', () => {
    service.list.and.returnValue(of([plan({ id: 'other-plan' }), plan({ id: 'workflow-plan' })]))
    component = new PlanCoordinationComponent(service, notification, {
      snapshot: { queryParamMap: { get: (key: string) => key === 'planId' ? 'workflow-plan' : null } },
    } as any)

    component.ngOnInit()

    expect(component.selectedPlan?.id).toBe('workflow-plan')
    expect(component.inspectorOpen).toBeTrue()
  })

  it('creates a trimmed preview with deduplicated criteria and constraints', () => {
    const preview = plan({ id: 'preview-1' })
    service.preview.and.returnValue(of(preview))
    component.previewForm = {
      title: '  Evidence response ',
      objective: ' Prepare a verified response ',
      successCriteria: 'Sources attached\nSources attached\nTests pass',
      deadlineAt: '',
      pursuitId: '',
      workflowId: '',
      constraints: 'Draft only\nDraft only',
      estimatedMinutes: 45,
      estimatedCostEur: 0,
    }

    component.createPreview()

    expect(service.preview).toHaveBeenCalledWith(jasmine.objectContaining({
      title: 'Evidence response',
      nodes: jasmine.arrayContaining([
        jasmine.objectContaining({ type: 'objective', title: 'Prepare a verified response', estimatedMinutes: 45 }),
        jasmine.objectContaining({ type: 'success_criterion', title: 'Sources attached' }),
        jasmine.objectContaining({ type: 'constraint', title: 'Draft only' }),
      ]),
    }))
    expect(component.selectedPlan?.id).toBe('preview-1')
    expect(component.inspectorOpen).toBeTrue()
  })

  it('accepts only the currently loaded revision and digest', () => {
    const accepted = plan({ status: 'accepted', revision: 1 })
    service.accept.and.returnValue(of(accepted))
    component.selectedPlan = plan()

    component.acceptSelected()

    expect(service.accept).toHaveBeenCalledWith('plan-1', {
      expectedRevision: 1,
      expectedDigest: 'digest-1',
    })
    expect(component.selectedPlan?.status).toBe('accepted')
  })

  it('creates a new revision when an accepted plan is replanned', () => {
    const accepted = plan({ status: 'accepted', revision: 2, planDigest: 'digest-2' })
    const repaired = plan({ status: 'draft', revision: 3, planDigest: 'digest-3' })
    service.replan.and.returnValue(of(repaired))
    component.selectedPlan = accepted
    component.replanOpen = true
    component.replanReason = 'External deadline moved'
    component.replanConstraints = 'Use the new hearing date'

    component.replanSelected()

    expect(service.replan).toHaveBeenCalledWith('plan-1', jasmine.objectContaining({
      expectedRevision: 2,
      expectedDigest: 'digest-2',
      reason: 'External deadline moved',
      trigger: 'owner_requested',
      nodes: jasmine.arrayContaining([
        jasmine.objectContaining({ type: 'constraint', title: 'Use the new hearing date' }),
      ]),
    }))
    expect(component.selectedPlan?.revision).toBe(3)
    expect(component.replanOpen).toBeFalse()
  })

  it('keeps API failure distinct from an empty plan list', () => {
    service.list.and.returnValue(throwError(() => new HttpErrorResponse({
      status: 503,
      error: { error: 'plan graph store unavailable' },
    })))

    component.refresh()

    expect(component.loading).toBeFalse()
    expect(component.errorMessage).toBe('plan graph store unavailable')
    expect(component.plans).toEqual([])
  })
})

describe('PlanCoordinationComponent rendering', () => {
  let fixture: ComponentFixture<PlanCoordinationComponent>
  let renderedService: jasmine.SpyObj<PlanGraphService>

  beforeEach(async () => {
    renderedService = jasmine.createSpyObj<PlanGraphService>('PlanGraphService', ['list', 'get', 'preview', 'accept', 'replan'])
    renderedService.list.and.returnValue(of([]))
    await TestBed.configureTestingModule({
      declarations: [PlanCoordinationComponent],
      imports: [
        CommonModule,
        FormsModule,
        BrowserAnimationsModule,
        ControlRoomModule,
        NzButtonModule,
        NzDrawerModule,
        NzIconModule,
        NzNotificationModule,
        NzSpinModule,
      ],
      providers: [
        {
          provide: NZ_ICONS,
          useValue: [
            AuditOutline,
            BranchesOutline,
            CheckCircleOutline,
            CloseOutline,
            DownOutline,
            LinkOutline,
            NodeIndexOutline,
            PauseCircleOutline,
            ProfileOutline,
            ReloadOutline,
            RightOutline,
            SafetyCertificateOutline,
            UpOutline,
            WarningOutline,
          ],
        },
        { provide: PlanGraphService, useValue: renderedService },
        { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: { get: () => null } } } },
        { provide: NzNotificationService, useValue: jasmine.createSpyObj('NzNotificationService', ['success', 'error']) },
      ],
    }).compileComponents()
    fixture = TestBed.createComponent(PlanCoordinationComponent)
  })

  it('renders a calm immutable empty state and preview form', () => {
    fixture.detectChanges()

    const text = (fixture.nativeElement as HTMLElement).textContent || ''
    expect(text).toContain('No coordination plan exists yet')
    expect(text).toContain('Create plan preview')
    expect(renderedService.list).toHaveBeenCalledWith()
  })
})
