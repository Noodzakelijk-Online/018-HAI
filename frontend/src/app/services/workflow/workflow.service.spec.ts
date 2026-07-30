import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { IWorkflowFrameworkSelectionDecision } from '../../models/workflow.model.interface';
import { WorkflowService } from './workflow.service';

describe('WorkflowService framework provenance', () => {
  let service: WorkflowService;
  let http: HttpTestingController;

  const selection = {
    id: 'c595d075-5412-4e7f-bff4-1a9df360451a',
    selected: [],
  } as unknown as IWorkflowFrameworkSelectionDecision;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(WorkflowService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('finds an exact owner-scoped framework selection without inventing data', () => {
    let result: IWorkflowFrameworkSelectionDecision | undefined;
    service.frameworkSelection(` ${selection.id} `).subscribe((value) => (result = value));

    const request = http.expectOne((candidate) =>
      candidate.url === '/api/v1/framework-registry/selections' &&
      candidate.params.get('limit') === '200'
    );
    expect(request.request.method).toBe('GET');
    request.flush({ selections: [{ id: 'other-selection' }, selection] });

    expect(result).toBe(selection);
  });

  it('returns undefined when the exact selection is outside available registry history', () => {
    let result: IWorkflowFrameworkSelectionDecision | undefined = selection;
    service.frameworkSelection(selection.id).subscribe((value) => (result = value));

    const request = http.expectOne((candidate) =>
      candidate.url === '/api/v1/framework-registry/selections' &&
      candidate.params.get('limit') === '200'
    );
    request.flush({ selections: [] });

    expect(result).toBeUndefined();
  });
});
