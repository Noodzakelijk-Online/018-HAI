import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';

import { AutomationsService } from './automations.service';

describe('AutomationsService', () => {
  let service: AutomationsService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(AutomationsService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('uses PATCH to reorder shared automation configuration', () => {
    service.swapAutomations('first', 'second').subscribe();

    const request = http.expectOne('/api/v1/automation/swap/first/second');
    expect(request.request.method).toBe('PATCH');
    expect(request.request.body).toEqual({});
    request.flush(null);
  });
});
