import { TestBed } from '@angular/core/testing';
import { CanActivateFn, Router } from '@angular/router';
import { of } from 'rxjs';

import { authGuard } from './auth.guard';
import { AUTH_SERVICE_TOKEN } from '../auth.service.token';

describe('authGuard', () => {
  const executeGuard: CanActivateFn = (...guardParameters) => 
      TestBed.runInInjectionContext(() => authGuard(...guardParameters));

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        { provide: AUTH_SERVICE_TOKEN, useValue: { loggedIn: () => of(false) } },
        { provide: Router, useValue: jasmine.createSpyObj<Router>('Router', ['createUrlTree']) },
      ],
    });
  });

  it('should be created', () => {
    expect(executeGuard).toBeTruthy();
  });

  it('preserves the requested internal route when authentication is required', (done) => {
    const router = TestBed.inject(Router) as jasmine.SpyObj<Router>;
    const returnTree = {} as any;
    router.createUrlTree.and.returnValue(returnTree);

    (executeGuard({} as any, { url: '/connected-sources?source=gmail' } as any) as any).subscribe((result: unknown) => {
      expect(result).toBe(returnTree);
      expect(router.createUrlTree).toHaveBeenCalledWith(
        ['/login'],
        { queryParams: { returnUrl: '/connected-sources?source=gmail' } },
      );
      done();
    });
  });
});
