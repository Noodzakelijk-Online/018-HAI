import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';
import { BackgroundOperationsService } from './background-operations.service';

describe('BackgroundOperationsService', () => {
  let service: BackgroundOperationsService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(BackgroundOperationsService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads the ledger dashboard and filtered operations in one request', () => {
    service.overview({ status: 'blocked' }).subscribe((overview) => {
      expect(overview.dashboard.blocked).toBe(1);
      expect(overview.operations[0].status).toBe('blocked');
    });

    const request = http.expectOne('/api/v1/operations/overview?status=blocked');
    expect(request.request.method).toBe('GET');
    request.flush({ dashboard: { blocked: 1 }, operations: [{ status: 'blocked' }] });
  });
});
