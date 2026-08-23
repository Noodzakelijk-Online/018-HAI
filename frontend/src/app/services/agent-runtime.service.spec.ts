import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';
import { AgentRuntimeService } from './agent-runtime.service';

describe('AgentRuntimeService', () => {
  let service: AgentRuntimeService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(AgentRuntimeService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads the runtime overview in one request', () => {
    service.overview().subscribe((overview) => {
      expect(overview.runtimes[0].id).toBe('deepseek-harness');
      expect(overview.health[0].runtimeId).toBe('deepseek-harness');
    });

    const request = http.expectOne('/api/v1/agent-runtimes/overview');
    expect(request.request.method).toBe('GET');
    request.flush({
      runtimes: [{ id: 'deepseek-harness' }],
      health: [{ runtimeId: 'deepseek-harness' }],
    });
  });
});
