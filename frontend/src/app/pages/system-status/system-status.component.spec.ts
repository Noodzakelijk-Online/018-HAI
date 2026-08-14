import { fakeAsync, tick } from '@angular/core/testing';
import { Subject, of } from 'rxjs';
import { ISystemReadiness } from '../../models/system-status.model.interface';
import { ISystemStatusService } from '../../services/system-status/system-status.service.interface';
import { SystemStatusComponent } from './system-status.component';

describe('SystemStatusComponent polling', () => {
  const readiness: ISystemReadiness = {
    status: 'ready',
    service: 'backend',
    summary: { ok: 1, warn: 0, fail: 0 },
    checks: [{ name: 'server.port', severity: 'ok', detail: 'listening' }],
  };

  function service(readinessResult = of(readiness)): jasmine.SpyObj<ISystemStatusService> {
    const result = jasmine.createSpyObj<ISystemStatusService>(
      'SystemStatusService',
      ['readiness', 'eventDelivery', 'retryEventDelivery']
    );
    result.readiness.and.returnValue(readinessResult);
    result.eventDelivery.and.returnValue(of({
      pending: 0,
      deadLettered: 0,
      published: 0,
      recentFailures: [],
      checkedAt: new Date(0).toISOString(),
    }));
    return result;
  }

  it('polls healthy systems every two minutes and pauses completely while hidden', fakeAsync(() => {
    const statusService = service();
    const notification = jasmine.createSpyObj('NzNotificationService', ['error', 'success']);
    let hidden = false;
    const hiddenSpy = spyOnProperty(document, 'hidden', 'get').and.callFake(() => hidden);
    const component = new SystemStatusComponent(statusService, notification, document);

    component.ngOnInit();
    expect(statusService.readiness).toHaveBeenCalledTimes(1);

    tick(119999);
    expect(statusService.readiness).toHaveBeenCalledTimes(1);
    tick(1);
    expect(statusService.readiness).toHaveBeenCalledTimes(2);

    hidden = true;
    document.dispatchEvent(new Event('visibilitychange'));
    tick(240000);
    expect(statusService.readiness).toHaveBeenCalledTimes(2);

    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    expect(statusService.readiness).toHaveBeenCalledTimes(3);

    component.ngOnDestroy();
    hiddenSpy.and.callThrough();
  }));

  it('does not overlap readiness requests when a prior probe is still running', fakeAsync(() => {
    const response = new Subject<ISystemReadiness>();
    const statusService = service(response);
    const notification = jasmine.createSpyObj('NzNotificationService', ['error', 'success']);
    const component = new SystemStatusComponent(statusService, notification, document);

    component.ngOnInit();
    tick(360000);
    expect(statusService.readiness).toHaveBeenCalledTimes(1);

    response.next(readiness);
    response.complete();
    tick(120000);
    expect(statusService.readiness).toHaveBeenCalledTimes(2);

    component.ngOnDestroy();
  }));

  it('checks critical recovery every fifteen seconds', fakeAsync(() => {
    const notReady: ISystemReadiness = { ...readiness, status: 'not_ready' };
    const statusService = service(of(notReady));
    const notification = jasmine.createSpyObj('NzNotificationService', ['error', 'success']);
    const component = new SystemStatusComponent(statusService, notification, document);

    component.ngOnInit();
    tick(14999);
    expect(statusService.readiness).toHaveBeenCalledTimes(1);
    tick(1);
    expect(statusService.readiness).toHaveBeenCalledTimes(2);

    component.ngOnDestroy();
  }));
});
