import { Router } from '@angular/router'
import { of, throwError } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { AccountBridgesService } from '../../services/account-bridges.service'
import { AccountBridgesComponent } from './account-bridges.component'

describe('AccountBridgesComponent refresh', () => {
  it('keeps a visible recovery state when account bridge data cannot load', () => {
    const service = jasmine.createSpyObj<AccountBridgesService>('AccountBridgesService', ['bridges', 'feeds'])
    service.bridges.and.returnValue(throwError(() => new Error('gateway unavailable')))
    service.feeds.and.returnValue(of({ feeds: [] }))
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['error'])
    const router = jasmine.createSpyObj<Router>('Router', ['navigate'])
    const component = new AccountBridgesComponent(service, notification, router)

    component.refresh()

    expect(component.overviewLoadError).toContain('account bridges')
    expect(component.loading).toBeFalse()
  })
})
