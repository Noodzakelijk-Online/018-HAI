import { FormBuilder } from '@angular/forms';
import { of } from 'rxjs';
import { IPursuitAction, IPursuitDecision, IPursuitDetail, IPursuitLink } from '../../models/pursuit.model.interface';
import { PursuitsComponent } from './pursuits.component';

describe('PursuitsComponent action lanes', () => {
  let component: PursuitsComponent;
  let notification: jasmine.SpyObj<{
    info: (title: string, content: string) => void;
    success: (title: string, content: string) => void;
    error: (title: string, content: string) => void;
  }>;

  beforeEach(() => {
    notification = jasmine.createSpyObj('NzNotificationService', ['info', 'success', 'error']);
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

  it('uses the explicit candidate acceptance endpoint for an approved candidate decision', () => {
    const candidate = { id: 'candidate-1', riskLevel: 'medium' } as any;
    const detail = { pursuit: candidate } as IPursuitDetail;
    const pursuitService = (component as any).pursuitsService;
    pursuitService.acceptCandidate = jasmine.createSpy('acceptCandidate').and.returnValue(of(detail));
    component.selected = detail;
    spyOn(component, 'load');
    const decision: IPursuitDecision = {
      id: 'pursuit:candidate-1:candidate-review',
      decisionType: 'pursuit_candidate_review',
      riskLevel: 'medium',
      reason: 'Confirm this imported objective before planning work.',
    } as IPursuitDecision;

    (component as any).resolvePursuitCandidateReview(decision, true);

    expect(pursuitService.acceptCandidate).toHaveBeenCalledWith('candidate-1', {
      requiresReview: false,
      reviewReason: decision.reason,
    });
    expect(notification.success).toHaveBeenCalledWith('Candidate accepted', 'HAI converted the candidate into governed pursuit work.');
  });

  it('does not claim that routed candidate intake created governed work', () => {
    const pursuitService = (component as any).pursuitsService;
    pursuitService.routeIntake = jasmine.createSpy('routeIntake').and.returnValue(of({
      mode: 'candidate_created',
      createdCandidate: true,
      pursuitId: 'candidate-1',
      matches: [],
    }));
    spyOn(component, 'load');
    const selectPursuit = spyOn<any>(component as any, 'selectPursuitById');
    component.routedIntakeForm.patchValue({ input: 'An unmatched source signal' });

    component.routeIntake();

    expect(notification.info).toHaveBeenCalledWith(
      'Pursuit candidate needs review',
      'HAI recorded the unmatched input as a reviewable pursuit candidate. No workflow was created until an approver accepts it.'
    );
    expect(notification.success).not.toHaveBeenCalledWith('Pursuit candidate created', jasmine.anything());
    expect(selectPursuit).toHaveBeenCalledWith('candidate-1', true);
  });

  it('opens a linked pursuit from the relationship ledger', () => {
    const router = (component as any).router;
    router.navigate = jasmine.createSpy('navigate');
    const link: IPursuitLink = {
      id: 'link-1',
      pursuitId: 'pursuit-1',
      linkType: 'pursuit',
      linkId: 'pursuit-2',
      relationship: 'related_case',
      confidence: 0.9,
      createdAt: '2026-07-14T00:00:00Z',
    };

    component.openLinkedPursuit(link);

    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: { selected: 'pursuit-2' } });
  });
});
