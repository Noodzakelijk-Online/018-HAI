import { FormBuilder } from '@angular/forms';
import { IPursuitAction, IPursuitDetail } from '../../models/pursuit.model.interface';
import { PursuitsComponent } from './pursuits.component';

describe('PursuitsComponent action lanes', () => {
  let component: PursuitsComponent;
  let notification: jasmine.SpyObj<{ info: (title: string, content: string) => void }>;

  beforeEach(() => {
    notification = jasmine.createSpyObj('NzNotificationService', ['info']);
    component = new PursuitsComponent(
      new FormBuilder(),
      {} as any,
      {} as any,
      {} as any,
      notification as any,
      {} as any,
      {} as any
    );
  });

  it('opens the first action in the selected operational lane', () => {
    const action: IPursuitAction = {
      label: 'Prepare the evidence index',
      owner: 'VA',
      riskLevel: 'low',
      requiresApproval: false,
      reason: 'The evidence list is ready for preparation.',
    };
    component.selected = {
      actionQueues: {
        needsRobert: [],
        vaReady: [action],
        systemReady: [],
        waiting: [],
      },
    } as unknown as IPursuitDetail;
    spyOn(component, 'openAction');

    component.openDigestLane('va');

    expect(component.openAction).toHaveBeenCalledWith(action);
    expect(notification.info).not.toHaveBeenCalled();
  });

  it('explains when the selected operational lane has no action', () => {
    component.selected = {
      actionQueues: {
        needsRobert: [],
        vaReady: [],
        systemReady: [],
        waiting: [],
      },
    } as unknown as IPursuitDetail;
    spyOn(component, 'openAction');

    component.openDigestLane('system');

    expect(component.openAction).not.toHaveBeenCalled();
    expect(notification.info).toHaveBeenCalledWith('System-ready lane', 'There is no system-ready action to open for this pursuit.');
  });
});
