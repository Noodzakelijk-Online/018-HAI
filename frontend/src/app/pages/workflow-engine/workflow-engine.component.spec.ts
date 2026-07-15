import { FormBuilder } from '@angular/forms';
import { convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import { WorkflowEngineComponent } from './workflow-engine.component';

describe('WorkflowEngineComponent', () => {
  function createComponent(): {
    component: WorkflowEngineComponent;
    pursuitService: jasmine.SpyObj<any>;
    notification: jasmine.SpyObj<any>;
  } {
    const workflowService = jasmine.createSpyObj('workflowService', ['overview', 'dashboard', 'items', 'approvals']);
    const pursuitService = jasmine.createSpyObj('pursuitService', ['intake', 'routeIntake', 'match']);
    const notification = jasmine.createSpyObj('notification', ['success', 'info', 'error']);
    const route = { snapshot: { queryParamMap: convertToParamMap({}) } } as any;
    const router = jasmine.createSpyObj('router', ['navigate']);
    const component = new WorkflowEngineComponent(new FormBuilder(), workflowService, pursuitService, notification, route, router);
    spyOn(component, 'refresh');
    return { component, pursuitService, notification };
  }

  it('does not claim that candidate intake created governed work', () => {
    const { component, pursuitService, notification } = createComponent();
    pursuitService.routeIntake.and.returnValue(of({
      mode: 'candidate_created',
      createdCandidate: true,
      pursuitId: 'candidate-1',
      matches: [],
    }));

    component.intake();

    expect(notification.info).toHaveBeenCalledWith(
      'Pursuit candidate needs review',
      'HAI recorded the unmatched input as a reviewable pursuit candidate. No workflow was created until an approver accepts it.'
    );
    expect(notification.success).not.toHaveBeenCalledWith(
      'Pursuit candidate created',
      jasmine.anything()
    );
    expect(component.selected).toBeUndefined();
  });
});
