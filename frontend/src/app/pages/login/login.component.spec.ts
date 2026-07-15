import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { LoginComponent } from './login.component';

describe('LoginComponent registration', () => {
  function createComponent(): { component: LoginComponent; auth: jasmine.SpyObj<any>; notification: jasmine.SpyObj<any> } {
    const auth = jasmine.createSpyObj('AuthService', ['login', 'register', 'requestPasswordReset', 'confirmPasswordReset']);
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error', 'info', 'create']);
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const component = new LoginComponent(new FormBuilder(), notification as NzNotificationService, router, auth);
    component.ngOnInit();
    return { component, auth, notification };
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
