import { fakeAsync, tick } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { ISystemReadiness } from '../../models/system-status.model.interface';
import { ISystemStatusService } from '../../services/system-status/system-status.service.interface';
import { SystemStatusComponent } from './system-status.component';

describe('SystemStatusComponent', () => {
  let readiness: Subject<ISystemReadiness>;
  let service: jasmine.SpyObj<ISystemStatusService>;
  let component: SystemStatusComponent;

  const sampleReadiness: ISystemReadiness = {
    status: 'ready',
    service: 'backend',
    summary: { ok: 1, warn: 0, fail: 0 },
    checks: [{ name: 'server.health', severity: 'ok', detail: 'ready' }],
  };

  beforeEach(() => {
    readiness = new Subject<ISystemReadiness>();
    service = jasmine.createSpyObj<ISystemStatusService>('SystemStatusService', ['readiness']);
    service.readiness.and.returnValue(readiness.asObservable());
    component = new SystemStatusComponent(service, jasmine.createSpyObj('Notification', ['error']));
  });

  it('does not overlap a slow readiness request', () => {
    component.refresh();
    component.refresh(true);
    expect(service.readiness).toHaveBeenCalledTimes(1);

    readiness.next(sampleReadiness);
    readiness.complete();
    component.refresh(true);
    expect(service.readiness).toHaveBeenCalledTimes(2);
  });

  it('skips polling while hidden and refreshes when the page becomes visible', fakeAsync(() => {
    let hidden = false;
    spyOnProperty(document, 'hidden', 'get').and.callFake(() => hidden);
    const refresh = spyOn(component, 'refresh');

    component.ngOnInit();
    expect(refresh).toHaveBeenCalledTimes(1);

    hidden = true;
    tick(15000);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).toHaveBeenCalledTimes(1);

    hidden = false;
    document.dispatchEvent(new Event('visibilitychange'));
    expect(refresh).toHaveBeenCalledTimes(2);
    component.ngOnDestroy();
  }));

  it('waits one minute between visible readiness refreshes', fakeAsync(() => {
    spyOnProperty(document, 'hidden', 'get').and.returnValue(false);
    const refresh = spyOn(component, 'refresh');

    component.ngOnInit();
    expect(refresh).toHaveBeenCalledTimes(1);

    tick(15000);
    expect(refresh).toHaveBeenCalledTimes(1);

    tick(45000);
    expect(refresh).toHaveBeenCalledTimes(2);
    component.ngOnDestroy();
  }));
});
