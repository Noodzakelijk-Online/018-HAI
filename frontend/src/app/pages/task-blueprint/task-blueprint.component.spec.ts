import { FormBuilder } from '@angular/forms';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { TaskBlueprintComponent } from './task-blueprint.component';

describe('TaskBlueprintComponent pursuit context', () => {
  function createComponent(pursuitId: string = ''): { component: TaskBlueprintComponent; router: jasmine.SpyObj<Router> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const taskPlans = jasmine.createSpyObj('TaskPlanService', ['logs', 'reviewQueue', 'resolveReviewItem']);
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
        {} as NzNotificationService,
        router,
        route,
        { mode: () => 'light' } as any,
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
});
