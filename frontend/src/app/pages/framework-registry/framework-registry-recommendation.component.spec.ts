import { CommonModule } from '@angular/common';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { IFrameworkSelectionDecision } from '../../models/framework-registry.model.interface';
import { FrameworkRegistryRecommendationComponent } from './framework-registry-recommendation.component';

describe('FrameworkRegistryRecommendationComponent', () => {
  let fixture: ComponentFixture<FrameworkRegistryRecommendationComponent>;
  let component: FrameworkRegistryRecommendationComponent;

  const selection: IFrameworkSelectionDecision = {
    id: 'selection-1',
    taskPlanId: 'plan-1',
    createdAt: '2026-07-30T10:00:00Z',
    catalogVersion: 'v1',
    catalogDigest: 'a'.repeat(64),
    selectorAlgorithmVersion: 'selector-v3',
    effectivePreferenceDigest: 'b'.repeat(64),
    constitutionDigest: 'c'.repeat(64),
    lifeDomain: 'legal',
    needOrCommitment: 'respond to deadline',
    selected: [{
      id: 'truth-evidence',
      version: '1.0.0',
      name: 'Knowledge and truth',
      family: 'knowledge',
      score: 9,
      reasons: ['evidence is required'],
      maximumAutonomyLevel: 3,
      authorityRequirement: 'draft only',
      evidenceRequirements: ['source link'],
      evaluationMethod: ['claim support'],
    }],
    conflicts: [],
    requiredAgents: ['evidence_agent'],
    maximumAutonomyLevel: 2,
    authoritySummary: 'Draft only',
    requiresApproval: true,
    approvalReasons: ['legal communication'],
    evidenceRequirements: ['source link'],
    completionCriteria: ['draft is source grounded'],
    learningPlan: [],
    contextRequirements: ['case documents'],
    selectionReason: 'Legal work needs evidence and approval.',
    constitutionVersion: 1,
    constitutionSource: 'builtin-robert-constitution-v1:v1',
  };

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      declarations: [FrameworkRegistryRecommendationComponent],
      imports: [CommonModule],
      schemas: [NO_ERRORS_SCHEMA],
    }).compileComponents();

    fixture = TestBed.createComponent(FrameworkRegistryRecommendationComponent);
    component = fixture.componentInstance;
  });

  it('announces loading instead of presenting a false empty state', () => {
    component.loading = true;
    fixture.detectChanges();

    const root = fixture.nativeElement as HTMLElement;
    const status = root.querySelector<HTMLElement>('[role="status"]');

    expect(status?.textContent).toContain('Loading the latest recommendation');
    expect(root.textContent).not.toContain('No recommendation yet');
  });

  it('distinguishes an unavailable history endpoint from no recommendation', () => {
    component.unavailableMessage = 'Selection history is unavailable.';
    fixture.detectChanges();

    const root = fixture.nativeElement as HTMLElement;
    const alert = root.querySelector<HTMLElement>('[role="alert"]');

    expect(alert?.textContent).toContain('Recommendation history unavailable');
    expect(alert?.textContent).toContain('Selection history is unavailable.');
    expect(root.textContent).not.toContain('No recommendation yet');
  });

  it('opens a selected framework through a named keyboard-operable button', () => {
    const inspectSpy = spyOn(component.inspectFramework, 'emit');
    component.selection = selection;
    fixture.detectChanges();

    const button = fixture.nativeElement.querySelector(
      'button.selected-framework'
    ) as HTMLButtonElement | null;

    expect(button?.getAttribute('aria-label')).toBe(
      'Inspect Knowledge and truth framework'
    );
    button?.click();
    expect(inspectSpy).toHaveBeenCalledOnceWith('truth-evidence');
  });

  it('labels a retained recommendation when history refresh failed', () => {
    component.selection = selection;
    component.staleMessage = 'Selection history is unavailable.';
    fixture.detectChanges();

    const stale = (fixture.nativeElement as HTMLElement).querySelector(
      '.recommendation-stale[role="status"]'
    );

    expect(stale?.textContent).toContain('Selection history is unavailable.');
    expect(stale?.textContent).toContain('last successfully loaded recommendation');
  });

  it('exposes the complete selection and reproducibility contract in Advanced view', () => {
    component.selection = {
      ...selection,
      conflicts: [{
        selectedId: 'truth-evidence',
        skippedId: 'ungrounded-answering',
        reason: 'Evidence discipline takes precedence.',
      }],
      learningPlan: ['Record only verified corrections'],
    };
    component.advanced = true;
    fixture.detectChanges();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    [
      'Selected framework contracts',
      'evidence is required',
      'claim support',
      'Conflict resolution',
      'ungrounded-answering',
      'Context requirements',
      'case documents',
      'Learning plan',
      'Record only verified corrections',
      'Reproducibility and audit contract',
      'selection-1',
      'plan-1',
      'selector-v3',
      'builtin-robert-constitution-v1:v1',
      'a'.repeat(64),
      'b'.repeat(64),
      'c'.repeat(64),
    ].forEach((value) => expect(text).toContain(value));
  });
});
