import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';
import { ContextMemoryService } from './context-memory.service';

describe('ContextMemoryService', () => {
  let service: ContextMemoryService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(ContextMemoryService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('requests a bounded local semantic reindex', () => {
    service.reindexSemantic(500).subscribe((result) => {
      expect(result.enabled).toBeTrue();
      expect(result.indexed).toBe(2);
    });

    const request = http.expectOne('/api/v1/memory/semantic/reindex?limit=100');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({});
    request.flush({
      enabled: true,
      attempted: 2,
      indexed: 2,
      failed: 0,
      deferred: 0,
      explanation: 'Two visible records were indexed locally.',
    });
  });
});
