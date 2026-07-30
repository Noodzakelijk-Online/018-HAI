import { FormBuilder } from '@angular/forms';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { TaskBlueprintComponent } from './task-blueprint.component';

describe('TaskBlueprintComponent pursuit context', () => {
  function createComponent(pursuitId: string = ''): { component: TaskBlueprintComponent; router: jasmine.SpyObj<Router> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const taskPlans = jasmine.createSpyObj('TaskPlanService', ['logs', 'reviewQueue', 'resolveReviewItem']);
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['info', 'warning', 'error']);
    taskPlans.logs.and.returnValue(of([]));
    taskPlans.reviewQueue.and.returnValue(of([]));
    const route = {
      queryParamMap: of(convertToParamMap(pursuitId ? { pursuitId } : {})),
    } as ActivatedRoute;

    return {
      component: new TaskBlueprintComponent(
        new FormBuilder(),
        taskPlans,
        {} as any,
        notification,
        router,
        route,
        { mode: () => 'light' } as any,
        { propose: jasmine.createSpy('propose') } as any,
        { propose: jasmine.createSpy('propose') } as any,
      ),
      router,
    };
  }

  it('shows query-scoped pursuit context and opens its detail view', () => {
    const pursuitId = '3ca4a3b5-84b2-4fcd-ae8d-e9f337e7250b';
    const { component, router } = createComponent(pursuitId);

    component.ngOnInit();
    component.openPursuitContext();

    expect(component.planForm.value.pursuitId).toBe(pursuitId);
    expect(component.contextExpanded).toBeTrue();
    expect(component.pursuitContextLabel()).toBe('Selected pursuit');
    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: { selected: pursuitId } });
  });

  it('opens advanced context instead of navigating when no pursuit is selected', () => {
    const { component, router } = createComponent();

    component.openPursuitContext();

    expect(component.contextExpanded).toBeTrue();
    expect(component.pursuitContextLabel()).toBe('No pursuit selected');
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('copies a reviewed typed draft only into editable success criteria', () => {
    const { component } = createComponent();
    component.typedProposal = {
      engine: 'pydantic-ai 2.13.0', modelId: 'qwen-local', requestDigest: 'digest', scope: 'draft only',
      proposal: { goal: 'Prepare a plan', successCriteria: ['Use evidence', 'Do not send'], nextSteps: ['Inspect'], risk: 'medium', requiresApproval: true, reasons: ['External impact'], uncertainties: [] },
    };

    component.useTypedProposalCriteria();

    expect(component.planForm.value.successCriteria).toBe('Use evidence\nDo not send');
    expect(component.contextExpanded).toBeTrue();
  });

  it('copies a reviewed CrewAI draft only into editable success criteria', () => {
    const { component } = createComponent();
    component.crewProposal = {
      engine: 'crewai 1.15.5', modelId: 'qwen-local', requestDigest: 'digest', scope: 'draft only',
      proposal: { goal: 'Prepare a plan', successCriteria: ['Use evidence', 'Do not send'], nextSteps: ['Inspect'], risk: 'medium', requiresApproval: true, reasons: ['External impact'], uncertainties: [] },
    };

    component.useCrewProposalCriteria();

    expect(component.planForm.value.successCriteria).toBe('Use evidence\nDo not send');
    expect(component.contextExpanded).toBeTrue();
  });
});
