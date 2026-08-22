import { Router } from '@angular/router'
import { of, Subject, throwError } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { ControlCenterComponent } from './control-center.component'

describe('ControlCenterComponent dashboard refresh', () => {
  function createComponent() {
    const workflow = new Subject<any>()
    const ambient = new Subject<any>()
    const pursuits = new Subject<any>()
    const workflowService = jasmine.createSpyObj('WorkflowService', ['dashboard'])
    const pursuitService = jasmine.createSpyObj('PursuitService', ['dashboard'])
    const ambientService = jasmine.createSpyObj('AmbientService', ['overview'])
    workflowService.dashboard.and.returnValue(workflow.asObservable())
    pursuitService.dashboard.and.returnValue(pursuits.asObservable())
    ambientService.overview.and.returnValue(ambient.asObservable())

    const automationsService = jasmine.createSpyObj('AutomationsService', ['getAutomations', 'getHealthSummary'])
    const agentRuntimeService = jasmine.createSpyObj('AgentRuntimeService', ['overview'])
    const component = new ControlCenterComponent(
      automationsService,
      workflowService,
      pursuitService,
      jasmine.createSpyObj('AgentCycleService', ['run']),
      agentRuntimeService,
      ambientService,
      jasmine.createSpyObj('ContextMemoryService', ['list']),
      { mode: () => 'dark', toggle: () => 'light', label: () => 'Dark', icon: () => 'moon' } as any,
      jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['success', 'error', 'warning']),
      jasmine.createSpyObj<Router>('Router', ['navigate'])
    )
    const rebuild = spyOn<any>(component, 'rebuildViewModel')
    return { component, workflow, ambient, pursuits, workflowService, pursuitService, ambientService, automationsService, agentRuntimeService, rebuild }
  }

  it('coalesces the basic dashboard into one in-flight batch and one view rebuild', () => {
    const { component, workflow, ambient, pursuits, workflowService, pursuitService, ambientService, rebuild } = createComponent()

    component.refresh()
    component.refresh()

    expect(workflowService.dashboard).toHaveBeenCalledTimes(1)
    expect(ambientService.overview).toHaveBeenCalledTimes(1)
    expect(pursuitService.dashboard).toHaveBeenCalledTimes(1)
    expect(component.loading).toBeTrue()
    expect(rebuild).not.toHaveBeenCalled()

    workflow.next({ attention: [] })
    workflow.complete()
    ambient.next({ needs: [] })
    ambient.complete()
    pursuits.next({})
    pursuits.complete()

    expect(component.loading).toBeFalse()
    expect(rebuild).toHaveBeenCalledTimes(1)
  })

  it('keeps prior dashboard state and exposes a partial refresh failure', () => {
    const { component, workflow, ambient, pursuits, rebuild } = createComponent()
    const priorWorkflow = { attention: [{ id: 'existing-item' }] }
    component.workflowDashboard = priorWorkflow as any

    component.refresh()

    workflow.error(new Error('workflow unavailable'))
    ambient.next({ needs: [] })
    ambient.complete()
    pursuits.error(new Error('pursuits unavailable'))

    expect(component.workflowDashboard).toBe(priorWorkflow as any)
    expect(component.overviewLoadError).toContain('workflow')
    expect(component.overviewLoadError).toContain('pursuits')
    expect(component.loading).toBeFalse()
    expect(rebuild).toHaveBeenCalledTimes(1)
  })

  it('retains diagnostics and identifies the unavailable diagnostic lane', () => {
    const { component, automationsService, agentRuntimeService } = createComponent()
    const existingAutomation = { id: 'automation-1', name: 'Existing automation', position: 1 } as any
    component.automations = [existingAutomation]
    component.diagnosticsLoaded = true
    automationsService.getAutomations.and.returnValue(throwError(() => new Error('gateway unavailable')))
    automationsService.getHealthSummary.and.returnValue(of({ total: 1, healthy: 1, warning: 0, degraded: 0, broken: 0, unknown: 0 }))
    agentRuntimeService.overview.and.returnValue(of({ runtimes: [], health: [] }))

    component.loadDiagnosticsData(true)

    expect(component.automations).toEqual([existingAutomation])
    expect(component.diagnosticsLoadError).toContain('automations')
    expect(component.diagnosticsLoaded).toBeTrue()
  })
})
