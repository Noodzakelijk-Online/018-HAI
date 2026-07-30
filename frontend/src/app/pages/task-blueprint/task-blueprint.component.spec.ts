import { FormBuilder } from '@angular/forms';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { TaskBlueprintComponent } from './task-blueprint.component';
import { IFrameworkSelectionDecision } from '../../models/framework-registry.model.interface';

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
        {} as any,
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

  it('opens the framework registry from the plan inspector', () => {
    const { component, router } = createComponent();

    component.openFrameworkRegistry();

    expect(router.navigate).toHaveBeenCalledWith(['/framework-registry']);
  });

  it('keeps the framework summary concise while preserving the total count', () => {
    const { component } = createComponent();
    const decision = {
      selected: [
        { id: 'human-sovereignty', name: 'Human Sovereignty' },
        { id: 'intake-triage', name: 'Intake and Triage' },
        { id: 'approval', name: 'Approval Governance' },
        { id: 'truth', name: 'Truth and Evidence' },
        { id: 'privacy', name: 'Privacy and Security' },
      ],
    } as IFrameworkSelectionDecision;

    expect(component.visibleFrameworks(decision).map((framework) => framework.id)).toEqual([
      'human-sovereignty',
      'intake-triage',
      'approval',
    ]);
    expect(component.additionalFrameworkCount(decision)).toBe(2);
    expect(component.visibleFrameworks(undefined)).toEqual([]);
    expect(component.additionalFrameworkCount(undefined)).toBe(0);
  });

  it('summarizes real validation results and keeps unrecorded planned gates not run', () => {
    const { component } = createComponent();
    component.plan = {
      validationPlan: {
        steps: ['Run deterministic checks'],
        successCriteria: ['Deliverable exists', 'No unsupported claims'],
        frameworkEvidenceRequirements: ['Record source evidence'],
        frameworkCompletionCriteria: ['Record approval outcome'],
        frameworkAssuranceCriteria: ['Measure framework outcomes longitudinally'],
        failurePolicy: 'Escalate failed gates',
        completionGate: 'Every gate must pass',
      },
      validationResult: {
        passed: false,
        status: 'failed',
        checked: ['Deliverable exists'],
        failures: ['Record source evidence'],
        criteria: [
          {
            criterion: 'Deliverable exists',
            kind: 'task_success',
            status: 'passed',
            evidence: ['artifact://deliverable'],
          },
          {
            criterion: 'Record source evidence',
            kind: 'framework_evidence',
            status: 'failed',
            evidence: [],
            failure: 'No source evidence was recorded.',
          },
          {
            criterion: 'Measure framework outcomes longitudinally',
            kind: 'framework_assurance',
            status: 'not_applicable',
            evidence: ['framework-registry://evaluation'],
            applicabilityReason:
              'Evaluated by registry assurance and longitudinal evaluation, not by each task run.',
          },
        ],
        nextAction: 'Collect evidence',
        attemptNumber: 1,
      },
    } as any;

    const criteria = component.structuredValidationCriteria();

    expect(criteria.map((criterion) => criterion.criterion)).toEqual([
      'Deliverable exists',
      'Record source evidence',
      'Measure framework outcomes longitudinally',
      'No unsupported claims',
      'Record approval outcome',
    ]);
    expect(component.validationCount('passed')).toBe(1);
    expect(component.validationCount('failed')).toBe(1);
    expect(component.validationCount('not_run')).toBe(2);
    expect(component.validationCount('not_applicable')).toBe(1);
    expect(component.validationStatusLabel()).toBe('Failed');
    expect(component.validationStatusClass()).toBe('validation-state--failed');
  });

  it('distinguishes complete, partial, and absent validation without inventing evidence', () => {
    const { component } = createComponent();

    expect(component.validationStatusLabel()).toBe('Not run');
    expect(component.structuredValidationCriteria()).toEqual([]);

    component.plan = {
      validationPlan: {
        steps: [],
        successCriteria: ['Result is verified'],
        frameworkEvidenceRequirements: [],
        frameworkCompletionCriteria: [],
        frameworkAssuranceCriteria: [],
        failurePolicy: 'Retry',
        completionGate: 'All pass',
      },
      validationResult: {
        passed: false,
        status: 'pending',
        checked: [],
        failures: [],
        criteria: [],
        nextAction: 'Run validation',
        attemptNumber: 0,
      },
    } as any;

    expect(component.validationStatusLabel()).toBe('Not run');
    expect(component.structuredValidationCriteria()[0].evidence).toEqual([]);

    component.plan!.validationResult.criteria = [
      {
        criterion: 'Result is verified',
        kind: 'task_success',
        status: 'passed',
        evidence: ['verification://result'],
      },
      {
        criterion: 'Run deterministic checks',
        kind: 'system_check',
        status: 'not_run',
        evidence: [],
      },
    ];

    expect(component.validationStatusLabel()).toBe('Not fully run');

    component.plan!.validationResult.criteria[1].status = 'passed';
    component.plan!.validationResult.criteria[1].evidence = ['test://suite'];

    expect(component.validationStatusLabel()).toBe('Passed');
    expect(component.validationStatusClass()).toBe('validation-state--passed');
  });

  it('opens the exact validation evidence from the basic summary', () => {
    const { component } = createComponent();

    component.inspectorMode = 'overview';
    component.openValidationEvidence();

    expect(component.inspectorMode).toBe('evidence');
    expect(component.validationKindLabel('task_success')).toBe('Task success');
    expect(component.validationKindLabel('framework_evidence')).toBe('Framework evidence');
    expect(component.validationKindLabel('framework_completion')).toBe('Framework completion');
    expect(component.validationKindLabel('system_check')).toBe('System check');
  });
});
