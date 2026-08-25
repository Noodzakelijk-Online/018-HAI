import { Subject } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { AccountBridgesComponent } from './account-bridges.component';

describe('AccountBridgesComponent', () => {
  function componentWith(service: any): AccountBridgesComponent {
    return new AccountBridgesComponent(
      service,
      jasmine.createSpyObj<NzNotificationService>('notification', ['success', 'error']),
      jasmine.createSpyObj('router', ['navigate']),
    );
  }

  it('prevents duplicate sync-all requests until the active request completes', () => {
    const pending = new Subject<any>();
    const service = jasmine.createSpyObj('AccountBridgesService', ['syncDue']);
    service.syncDue.and.returnValue(pending.asObservable());
    const component = componentWith(service);

    component.syncAll();
    component.syncAll();
    expect(service.syncDue).toHaveBeenCalledTimes(1);
    expect(component.syncingAll).toBeTrue();

    pending.complete();
    expect(component.syncingAll).toBeFalse();
  });

  it('opens Connected Sources for provider-native connection state', () => {
    const router = jasmine.createSpyObj('router', ['navigate']);
    const component = new AccountBridgesComponent(
      jasmine.createSpyObj('AccountBridgesService', ['syncDue']),
      jasmine.createSpyObj<NzNotificationService>('notification', ['success', 'error']),
      router,
    );

    component.openConnectedSources();

    expect(router.navigate).toHaveBeenCalledWith(['/connected-sources']);
  });
});
