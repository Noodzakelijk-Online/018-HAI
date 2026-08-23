import { TestBed, fakeAsync, tick } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { AuthService } from './auth.service';
import { provideHttpClient, withInterceptorsFromDi } from '@angular/common/http';

describe('AuthService', () => {
  let service: AuthService;
  let controller: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [], providers: [provideHttpClient(withInterceptorsFromDi()), provideHttpClientTesting()] });
    service = TestBed.inject(AuthService);
    controller = TestBed.inject(HttpTestingController);
  });

  afterEach(() => controller.verify());

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('fails a hanging authentication probe promptly', fakeAsync(() => {
    let authenticated: boolean | undefined;
    service.loggedIn().subscribe((result) => (authenticated = result));
    controller.expectOne('/api/v1/auth/is-user-authenticated');

    tick(AuthService.authenticationCheckTimeoutMs + 1);

    expect(authenticated).toBeFalse();
  }));
});
