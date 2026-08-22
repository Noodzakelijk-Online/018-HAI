import { Router } from '@angular/router'
import { Subject, of, throwError } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { IOperation } from '../../models/background-operations.model.interface'
import { BackgroundOperationsService } from '../../services/background-operations.service'
import { BackgroundOperationsComponent } from './background-operations.component'

describe('BackgroundOperationsComponent action submission', () => {
  function createComponent(service: jasmine.SpyObj<BackgroundOperationsService>) {
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['success', 'error', 'warning'])
    const router = jasmine.createSpyObj<Router>('Router', ['navigate'])
    const component = new BackgroundOperationsComponent(service, notification, router)
    spyOn(component, 'refresh')
    return { component, notification }
  }

  const operation = {
    id: '5c09e7a8-53dd-4f2c-9a74-3f16b706d0a7',
    title: 'Refresh local briefing',
  } as IOperation

  it('submits an approval only once while the first request is pending', () => {
    const approval = new Subject<IOperation>()
    const service = jasmine.createSpyObj<BackgroundOperationsService>('BackgroundOperationsService', ['approve'])
    service.approve.and.returnValue(approval.asObservable())
    const { component } = createComponent(service)

    component.approve(operation)
    component.approve(operation)

    expect(service.approve).toHaveBeenCalledTimes(1)
    expect(component.isOperationActionPending(operation)).toBeTrue()

    approval.next(operation)
    approval.complete()

    expect(component.isOperationActionPending(operation)).toBeFalse()
  })

  it('submits a safe-worker run only once while the first request is pending', () => {
    const run = new Subject<{ operation: IOperation; verified: boolean; failed: boolean }>()
    const service = jasmine.createSpyObj<BackgroundOperationsService>('BackgroundOperationsService', ['run'])
    service.run.and.returnValue(run.asObservable())
    const { component } = createComponent(service)

    component.run(operation)
    component.run(operation)

    expect(service.run).toHaveBeenCalledTimes(1)
    expect(component.isOperationActionPending(operation)).toBeTrue()

    run.next({ operation, verified: true, failed: false })
    run.complete()

    expect(component.isOperationActionPending(operation)).toBeFalse()
  })

  it('clears the pending marker after a rejected request so the operator can recover', () => {
    const approval = new Subject<IOperation>()
    const service = jasmine.createSpyObj<BackgroundOperationsService>('BackgroundOperationsService', ['approve'])
    service.approve.and.returnValue(approval.asObservable())
    const { component } = createComponent(service)

    component.approve(operation)
    approval.error(new Error('request rejected'))

    expect(component.isOperationActionPending(operation)).toBeFalse()
  })
})

describe('BackgroundOperationsComponent refresh', () => {
  it('keeps a visible recovery state when the operational dashboard cannot load', () => {
    const service = jasmine.createSpyObj<BackgroundOperationsService>('BackgroundOperationsService', ['dashboard', 'list', 'feeds'])
    service.dashboard.and.returnValue(throwError(() => new Error('gateway unavailable')))
    service.list.and.returnValue(of({ operations: [] }))
    service.feeds.and.returnValue(of({ feeds: [] }))
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['success', 'error', 'warning'])
    const router = jasmine.createSpyObj<Router>('Router', ['navigate'])
    const component = new BackgroundOperationsComponent(service, notification, router)

    component.refresh()

    expect(component.overviewLoadError).toContain('operational dashboard')
    expect(component.loading).toBeFalse()
  })

  it('does not start overlapping dashboard batches while a refresh is pending', () => {
    const dashboard = new Subject<any>()
    const operations = new Subject<any>()
    const feeds = new Subject<any>()
    const queuedDashboard = new Subject<any>()
    const queuedOperations = new Subject<any>()
    const queuedFeeds = new Subject<any>()
    const service = jasmine.createSpyObj<BackgroundOperationsService>('BackgroundOperationsService', ['dashboard', 'list', 'feeds'])
    service.dashboard.and.returnValues(dashboard.asObservable(), queuedDashboard.asObservable())
    service.list.and.returnValues(operations.asObservable(), queuedOperations.asObservable())
    service.feeds.and.returnValues(feeds.asObservable(), queuedFeeds.asObservable())
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['success', 'error', 'warning'])
    const router = jasmine.createSpyObj<Router>('Router', ['navigate'])
    const component = new BackgroundOperationsComponent(service, notification, router)

    component.refresh()
    component.refresh()

    expect(service.dashboard).toHaveBeenCalledTimes(1)
    expect(service.list).toHaveBeenCalledTimes(1)
    expect(service.feeds).toHaveBeenCalledTimes(1)

    dashboard.next({})
    dashboard.complete()
    operations.next({ operations: [] })
    operations.complete()
    feeds.next({ feeds: [] })
    feeds.complete()

    expect(service.dashboard).toHaveBeenCalledTimes(2)
    expect(service.list).toHaveBeenCalledTimes(2)
    expect(service.feeds).toHaveBeenCalledTimes(2)

    queuedDashboard.next({})
    queuedDashboard.complete()
    queuedOperations.next({ operations: [] })
    queuedOperations.complete()
    queuedFeeds.next({ feeds: [] })
    queuedFeeds.complete()

    expect(component.loading).toBeFalse()
  })
})
