import { FormBuilder } from '@angular/forms';
import { convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import {
  IWorkflowFrameworkSelectionDecision,
  IWorkflowFrameworkSelectionProvenance,
  IWorkflowRecord,
} from '../../models/workflow.model.interface';
import { WorkflowEngineComponent } from './workflow-engine.component';

describe('WorkflowEngineComponent', () => {
  const selectionId = 'c595d075-5412-4e7f-bff4-1a9df360451a';
  const taskPlanId = 'task-plan-42';
  const digest = 'a'.repeat(64);

  const provenance: IWorkflowFrameworkSelectionProvenance = {
    selectionDecisionId: selectionId,
    taskPlanId,
    catalogVersion: '2026.07.1',
    catalogDigest: digest,
    selectorAlgorithmVersion: 'chief-of-staff-v1',
    effectivePreferenceDigest: 'b'.repeat(64),
    constitutionVersion: 3,
    constitutionDigest: 'c'.repeat(64),
    constitutionSource: 'owner-constitution:v3',
  };

  const registryDecision: IWorkflowFrameworkSelectionDecision = {
    id: selectionId,
    taskPlanId,
    createdAt: '2026-07-30T10:00:00Z',
    catalogVersion: provenance.catalogVersion,
    catalogDigest: provenance.catalogDigest,
    selectorAlgorithmVersion: provenance.selectorAlgorithmVersion,
    effectivePreferenceDigest: provenance.effectivePreferenceDigest,
    constitutionDigest: provenance.constitutionDigest,
    lifeDomain: 'work',
    needOrCommitment: 'Complete governed work',
    selected: [
      {
        id: 'human-sovereignty',
        version: '1.0.0',
        name: 'Human sovereignty',
        family: 'governance',
        score: 100,
        reasons: ['Required safety overlay'],
        maximumAutonomyLevel: 2,
        authorityRequirement: 'owner',
        evidenceRequirements: ['selection decision'],
        evaluationMethod: ['policy check'],
      },
    ],
    conflicts: [],
    requiredAgents: ['planner'],
    maximumAutonomyLevel: 2,
    authoritySummary: 'Owner retains authority.',
    requiresApproval: false,
    approvalReasons: [],
    evidenceRequirements: ['selection decision'],
    completionCriteria: ['verified result'],
    learningPlan: ['Record only verified lessons.'],
    contextRequirements: ['Owner-scoped source records.'],
    selectionReason: 'Required protected framework.',
    constitutionVersion: provenance.constitutionVersion,
    constitutionSource: provenance.constitutionSource,
  };

  function workflowRecord(
    frameworkSelections: IWorkflowFrameworkSelectionProvenance[]
  ): IWorkflowRecord {
    return {
      item: {
        id: 'workflow-1',
        title: 'Prepare evidence',
        currentState: 'completed',
        taskType: 'legal',
        riskLevel: 'high',
        priorityScore: 90,
        confidence: 0.9,
        autonomyLevel: 'suggest',
        requiresApproval: true,
        approvalStatus: 'approved',
        retryCount: 0,
        maxRetries: 3,
        lastTaskPlanId: taskPlanId,
        archived: false,
        createdAt: '2026-07-30T09:00:00Z',
        updatedAt: '2026-07-30T10:00:00Z',
      },
      checklist: [],
      intake: [],
      matches: [],
      pursuits: [],
      evidence: [],
      openLoops: [],
      proposals: [],
      qualityGates: [],
      transitions: [],
      sourceLinks: [],
      decisions: [],
      events: [],
      frameworkSelections,
    };
  }

  function createComponent(): {
    component: WorkflowEngineComponent;
    workflowService: jasmine.SpyObj<any>;
    pursuitService: jasmine.SpyObj<any>;
    notification: jasmine.SpyObj<any>;
    router: jasmine.SpyObj<any>;
  } {
    const workflowService = jasmine.createSpyObj('workflowService', [
      'overview',
      'dashboard',
      'items',
      'approvals',
      'get',
      'frameworkSelection',
      'transition',
      'resolveApproval',
      'resolveInterruptedExecution',
      'resolveProposal',
      'updateChecklistItem',
      'runDue',
      'runDueOpenLoops',
      'recoverStaleClaims',
    ]);
    const pursuitService = jasmine.createSpyObj('pursuitService', ['intake', 'routeIntake', 'match']);
    const notification = jasmine.createSpyObj('notification', ['success', 'info', 'warning', 'error']);
    const route = { snapshot: { queryParamMap: convertToParamMap({}) } } as any;
    const router = jasmine.createSpyObj('router', ['navigate']);
    const component = new WorkflowEngineComponent(new FormBuilder(), workflowService, pursuitService, notification, route, router);
    spyOn(component, 'refresh');
    return { component, workflowService, pursuitService, notification, router };
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

  it('verifies present workflow provenance against the exact registry selection', () => {
    const { component, workflowService } = createComponent();
    workflowService.get.and.returnValue(of(workflowRecord([provenance])));
    workflowService.frameworkSelection.and.returnValue(of(registryDecision));

    component.open(workflowRecord([provenance]).item);

    expect(workflowService.frameworkSelection).toHaveBeenCalledOnceWith(selectionId);
    expect(component.frameworkProvenanceState).toBe('verified');
    expect(component.frameworkSelectionDecision?.selected).toEqual(registryDecision.selected);
    expect(component.frameworkGovernanceLabel).toBe('Governed selection verified');
  });

  it('shows missing provenance honestly and does not query registry history', () => {
    const { component, workflowService } = createComponent();

    component.applyWorkflowRecord(workflowRecord([]));

    expect(component.frameworkProvenanceState).toBe('missing');
    expect(component.frameworkGovernanceLabel).toBe('No framework selection recorded');
    expect(component.frameworkGovernanceSummary).toContain('cannot be confirmed');
    expect(workflowService.frameworkSelection).not.toHaveBeenCalled();
  });

  it('rejects invalid provenance even when the workflow state says completed', () => {
    const { component, workflowService } = createComponent();
    const invalid = {
      ...provenance,
      catalogDigest: 'not-a-digest',
    };

    component.applyWorkflowRecord(workflowRecord([invalid]));

    expect(component.selected?.item.currentState).toBe('completed');
    expect(component.frameworkProvenanceState).toBe('invalid');
    expect(component.frameworkGovernanceLabel).toBe('Framework provenance needs review');
    expect(component.frameworkGovernanceSummary).toContain('Do not treat');
    expect(component.frameworkProvenanceIssues).toContain(
      'Catalog digest is not a SHA-256 digest.'
    );
    expect(workflowService.frameworkSelection).not.toHaveBeenCalled();
  });

  it('navigates from workflow governance to the Framework Registry', () => {
    const { component, router } = createComponent();

    component.openFrameworkRegistry();

    expect(router.navigate).toHaveBeenCalledOnceWith(['/framework-registry']);
  });

  it('records an explicit operator review before retrying interrupted execution', () => {
    const { component, workflowService, notification } = createComponent();
    const interrupted = workflowRecord([]);
    interrupted.item.currentState = 'blocked';
    interrupted.item.recoveryStatus = 'needs_review';
    interrupted.item.blockedReason = 'Execution outcome is unknown.';
    const ready = workflowRecord([]);
    ready.item.currentState = 'ready';
    ready.item.recoveryStatus = 'retry_confirmed';
    component.applyWorkflowRecord(interrupted);
    component.interruptionForm.setValue({
      decision: 'retry',
      note: 'Checked the target system and confirmed that no external action occurred.',
      evidenceUri: '',
      evidenceLabel: '',
    });
    workflowService.resolveInterruptedExecution.and.returnValue(of(ready));

    component.resolveInterruptedExecution();

    expect(workflowService.resolveInterruptedExecution).toHaveBeenCalledOnceWith(
      interrupted.item.id,
      {
        decision: 'retry',
        note: 'Checked the target system and confirmed that no external action occurred.',
        evidenceUri: '',
        evidenceLabel: '',
        actor: 'operator',
      }
    );
    expect(component.selected?.item.currentState).toBe('ready');
    expect(notification.success).toHaveBeenCalled();
  });

  it('does not confirm an interrupted completion without linked evidence', () => {
    const { component, workflowService, notification } = createComponent();
    const interrupted = workflowRecord([]);
    interrupted.item.currentState = 'blocked';
    interrupted.item.recoveryStatus = 'needs_review';
    component.applyWorkflowRecord(interrupted);
    component.interruptionForm.setValue({
      decision: 'confirm_completed',
      note: 'The expected external result exists.',
      evidenceUri: '',
      evidenceLabel: '',
    });

    component.resolveInterruptedExecution();

    expect(workflowService.resolveInterruptedExecution).not.toHaveBeenCalled();
    expect(notification.error).toHaveBeenCalledWith(
      'Evidence required',
      'Add a source URI before confirming completion.'
    );
  });
});
