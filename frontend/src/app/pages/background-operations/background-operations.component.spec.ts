import { Router } from '@angular/router'
import { Subject, of } from 'rxjs'
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
