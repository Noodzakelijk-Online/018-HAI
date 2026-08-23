import { Subject } from 'rxjs';
import { ISystemReadiness } from '../../models/system-status.model.interface';
import { SystemStatusComponent } from './system-status.component';

describe('SystemStatusComponent', () => {
  it('does not overlap scheduled readiness refreshes', () => {
    const response = new Subject<ISystemReadiness>();
    const service = { readiness: jasmine.createSpy('readiness').and.returnValue(response.asObservable()) };
    const notification = { error: jasmine.createSpy('error') };
    const component = new SystemStatusComponent(service as never, notification as never);

    component.refresh();
    component.refresh(true);

    expect(service.readiness).toHaveBeenCalledTimes(1);

    response.next({
      status: 'ready', service: 'backend', summary: { ok: 0, warn: 0, fail: 0 }, checks: [],
    });
    component.refresh(true);

    expect(service.readiness).toHaveBeenCalledTimes(2);
  });
});
