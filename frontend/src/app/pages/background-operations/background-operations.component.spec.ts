import { HttpErrorResponse } from '@angular/common/http'
import { Observable, of } from 'rxjs'
import { BackgroundOperationsComponent } from './background-operations.component'

describe('BackgroundOperationsComponent', () => {
  function make(): BackgroundOperationsComponent {
    const service = {
      overview: () => of({ dashboard: {}, operations: [] }),
      feeds: () => of({ feeds: [] }),
      events: () => of({ events: [] }),
    }
    const notification = jasmine.createSpyObj('notification', ['error', 'success', 'warning', 'info'])
    const router = jasmine.createSpyObj('router', ['navigate'])
    return new BackgroundOperationsComponent(service as any, notification, router)
  }

  it('does not start a pass until an account feed is enabled', () => {
    const component = make()

    component.runBackground()

    expect(component.lastRunNotice).toContain('No enabled account feed')
    expect(component.lastRunError).toBe('')
    expect(component.running).toBeFalse()
  })

  it('reports whether a configured feed can be scanned', () => {
    const component = make()
    component.feeds = [{ name: 'Trello', provider: 'trello', accountLabel: 'Robert', sourceType: 'trello', enabled: true }]

    expect(component.hasEnabledFeeds()).toBeTrue()
  })

  it('cancels an obsolete refresh before starting another one', () => {
    let cancellations = 0
    const pending = () => new Observable(() => () => cancellations++)
    const service = {
      overview: pending,
      feeds: pending,
      events: () => of({ events: [] }),
    }
    const notification = jasmine.createSpyObj('notification', ['error', 'success', 'warning', 'info'])
    const router = jasmine.createSpyObj('router', ['navigate'])
    const component = new BackgroundOperationsComponent(service as any, notification, router)

    component.refresh()
    component.refresh()

    expect(cancellations).toBe(2)
  })

  it('gives an owner-permission recovery path for authorization failures', () => {
    const component = make()

    const message = component.backgroundRunError(new HttpErrorResponse({ status: 403 }))

    expect(message).toContain('owner account')
  })

  it('does not present backend unavailability as an operation failure', () => {
    const component = make()

    const message = component.backgroundRunError(new HttpErrorResponse({ status: 503 }))

    expect(message).toContain('temporarily unavailable')
    expect(message).toContain('were not changed')
  })
})
