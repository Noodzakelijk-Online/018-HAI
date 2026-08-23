import { FormBuilder } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { LoginComponent } from './login.component';

describe('LoginComponent registration', () => {
  function createComponent(returnUrl: string | null = null): { component: LoginComponent; auth: jasmine.SpyObj<any>; notification: jasmine.SpyObj<any>; router: jasmine.SpyObj<Router> } {
    const auth = jasmine.createSpyObj('AuthService', ['getCapabilities', 'openLocalPreview', 'login', 'register', 'requestPasswordReset', 'confirmPasswordReset']);
    auth.getCapabilities.and.returnValue(of({ googleLoginEnabled: false, passwordRecoveryEmailEnabled: false, localPreviewEnabled: false }));
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error', 'warning', 'info', 'create']);
    const router = jasmine.createSpyObj<Router>('Router', ['navigate', 'navigateByUrl']);
    const route = { snapshot: { queryParamMap: { get: (key: string) => key === 'returnUrl' ? returnUrl : null } } } as unknown as ActivatedRoute;
    const component = new LoginComponent(new FormBuilder(), notification as NzNotificationService, router, route, auth);
    component.ngOnInit();
    return { component, auth, notification, router };
  }

  it('creates an operator account and returns to login without authenticating', () => {
    const { component, auth, notification } = createComponent();
    auth.register.and.returnValue(of({ id: 'new-user', email: 'operator@example.com' }));
    component.toggleRegistration();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      password: 'local-passphrase-2026',
      confirmPassword: 'local-passphrase-2026',
    });

    component.submitForm();

    expect(auth.register).toHaveBeenCalledWith('operator@example.com', 'local-passphrase-2026');
    expect(auth.login).not.toHaveBeenCalled();
    expect(component.registrationMode).toBeFalse();
    expect(notification.success).toHaveBeenCalledWith('Account created', 'Your local operator account is ready. Sign in to continue.');
  });

  it('flags the confirmation field when it does not match before calling the IDP', () => {
    const { component, auth, notification } = createComponent();
    component.toggleRegistration();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      password: 'local-passphrase-2026',
      confirmPassword: 'different-passphrase-2026',
    });

    component.submitForm();

    expect(auth.register).not.toHaveBeenCalled();
    expect(component.validateForm.controls['confirmPassword'].hasError('mismatch')).toBeTrue();
    expect(notification.error).toHaveBeenCalledWith('Check your sign-up details', 'The confirmation password does not match.');
  });

  it('explains the password length requirement before calling the IDP', () => {
    const { component, auth, notification } = createComponent();
    component.toggleRegistration();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      password: 'short-pass',
      confirmPassword: 'short-pass',
    });

    component.submitForm();

    expect(auth.register).not.toHaveBeenCalled();
    expect(component.validateForm.controls['password'].hasError('minLength')).toBeTrue();
    expect(notification.error).toHaveBeenCalledWith('Check your sign-up details', 'Your password must contain at least 12 characters.');
  });

  it('marks an email as already registered when the IDP returns conflict', () => {
    const { component, auth, notification } = createComponent();
    auth.register.and.returnValue({ subscribe: (observer: any) => observer.error({ status: 409 }) });
    component.toggleRegistration();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      password: 'local-passphrase-2026',
      confirmPassword: 'local-passphrase-2026',
    });

    component.submitForm();

    expect(component.validateForm.controls['userName'].hasError('accountExists')).toBeTrue();
    expect(notification.error).toHaveBeenCalledWith('Account already exists', 'This email is already registered. Log in instead or use password recovery.');
  });

  it('requests a password reset without showing developer recovery instructions', () => {
    const { component, auth, notification } = createComponent();
    auth.getCapabilities.and.returnValue(of({ googleLoginEnabled: false, passwordRecoveryEmailEnabled: true, localPreviewEnabled: false }));
    component.ngOnInit();
    auth.requestPasswordReset.and.returnValue(of(void 0));
    component.validateForm.patchValue({ userName: 'operator@example.com' });

    component.showPasswordHelp();
    component.submitForm();

    expect(component.recoveryMode).toBeTrue();
    expect(component.recoveryStep).toBe('confirm');
    expect(auth.requestPasswordReset).toHaveBeenCalledWith('operator@example.com');
    expect(notification.success).toHaveBeenCalledWith(
      'Recovery requested',
      'If recovery is available for this account, a one-time reset code has been sent through its configured recovery channel.'
    );
  });

  it('opens the dashboard through the explicit local preview session', () => {
    const { component, auth, router } = createComponent('/connected-sources');
    auth.openLocalPreview.and.returnValue(of(void 0));

    component.openLocalPreview();

    expect(auth.openLocalPreview).toHaveBeenCalled();
    expect(router.navigateByUrl).toHaveBeenCalledWith('/connected-sources');
  });

  it('returns to the requested internal route after password login', () => {
    const { component, auth, router } = createComponent('/workflow-engine?view=advanced');
    auth.login.and.returnValue(of(void 0));
    component.validateForm.patchValue({ userName: 'operator@example.com', password: 'local-passphrase-2026' });

    component.submitForm();

    expect(router.navigateByUrl).toHaveBeenCalledWith('/workflow-engine?view=advanced');
  });

  it('rejects an unsafe login return path', () => {
    const { component, auth, router } = createComponent('//untrusted.example');
    auth.login.and.returnValue(of(void 0));
    component.validateForm.patchValue({ userName: 'operator@example.com', password: 'local-passphrase-2026' });

    component.submitForm();

    expect(router.navigateByUrl).toHaveBeenCalledWith('/control-center');
  });

  it('does not request a reset code when email recovery is unavailable', () => {
    const { component, auth, notification } = createComponent();
    component.validateForm.patchValue({ userName: 'operator@example.com' });

    component.showPasswordHelp();
    component.submitForm();

    expect(auth.requestPasswordReset).not.toHaveBeenCalled();
    expect(notification.warning).toHaveBeenCalledWith(
      'Email recovery is unavailable',
      'Configure a private SMTP delivery account for this HAI installation before requesting a reset code.'
    );
  });

  it('does not confirm a reset when the new passwords differ', () => {
    const { component, auth, notification } = createComponent();
    component.showPasswordHelp();
    component.showResetConfirmation();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      resetToken: 'reset-token',
      password: 'local-passphrase-2026',
      confirmPassword: 'different-passphrase-2026',
    });

    component.submitForm();

    expect(auth.confirmPasswordReset).not.toHaveBeenCalled();
    expect(component.validateForm.controls['confirmPassword'].hasError('mismatch')).toBeTrue();
    expect(notification.error).toHaveBeenCalledWith('Check your reset details', 'The confirmation password does not match.');
  });
});
