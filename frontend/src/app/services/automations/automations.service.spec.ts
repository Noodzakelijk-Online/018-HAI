import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule } from '@angular/common/http/testing';

import { AutomationsService } from './automations.service';

describe('AutomationsService', () => {
  let service: AutomationsService;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [HttpClientTestingModule] });
    service = TestBed.inject(AutomationsService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
