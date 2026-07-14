import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { IPursuitDashboardDecision } from '../../models/pursuit.model.interface';
import { CommandDashboardComponent } from './command-dashboard.component';

describe('CommandDashboardComponent pursuit candidate decisions', () => {
  function createComponent(): {
    component: CommandDashboardComponent;
    pursuits: jasmine.SpyObj<any>;
    notification: jasmine.SpyObj<any>;
  } {
    const pursuits = jasmine.createSpyObj('PursuitService', ['acceptCandidate', 'archive']);
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error']);
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const component = new CommandDashboardComponent(
      new FormBuilder(),
      {} as any,
      {} as any,
      {} as any,
      pursuits,
      notification as NzNotificationService,
      router,
    );
    spyOn(component, 'refreshPursuits');
    return { component, pursuits, notification };
  }

  function candidateDecision(riskLevel: string = 'medium'): IPursuitDashboardDecision {
    return {
      pursuit: { id: 'candidate-1', title: 'Imported legal correspondence' } as any,
      decision: {
        id: 'pursuit:candidate-1:candidate-review',
        decisionType: 'pursuit_candidate_review',
        status: 'pending',
        riskLevel,
        reason: 'Confirm this imported objective before planning work.',
        recommended: 'Accept the candidate',
        yesLabel: 'Accept',
        noLabel: 'Archive',
      } as any,
    } as IPursuitDashboardDecision;
  }

  it('accepts a candidate through the explicit approval endpoint', () => {
    const { component, pursuits, notification } = createComponent();
    const card = candidateDecision();
    pursuits.acceptCandidate.and.returnValue(of({}));

    component.resolveDashboardDecision(card, true);

    expect(component.canResolveDashboardDecision(card)).toBeTrue();
    expect(pursuits.acceptCandidate).toHaveBeenCalledWith('candidate-1', {
      requiresReview: false,
      reviewReason: 'Confirm this imported objective before planning work.',
    });
    expect(notification.success).toHaveBeenCalledWith('Candidate accepted', 'HAI converted the candidate into governed pursuit work.');
    expect(component.refreshPursuits).toHaveBeenCalled();
  });

  it('archives a rejected candidate without creating workflow work', () => {
    const { component, pursuits, notification } = createComponent();
    const card = candidateDecision('high');
    pursuits.archive.and.returnValue(of({}));

    component.resolveDashboardDecision(card, false);

    expect(pursuits.archive).toHaveBeenCalledWith('candidate-1', true);
    expect(pursuits.acceptCandidate).not.toHaveBeenCalled();
    expect(notification.success).toHaveBeenCalledWith('Candidate archived', 'The auto-created candidate was removed from active queues.');
    expect(component.refreshPursuits).toHaveBeenCalled();
  });
});
