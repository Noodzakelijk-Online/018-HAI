import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { of } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { LoginComponent } from './login.component';

describe('LoginComponent registration', () => {
  function createComponent(): { component: LoginComponent; auth: jasmine.SpyObj<any>; notification: jasmine.SpyObj<any> } {
    const auth = jasmine.createSpyObj('AuthService', ['login', 'register']);
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

  it('rejects a mismatched confirmation before calling the IDP', () => {
    const { component, auth, notification } = createComponent();
    component.toggleRegistration();
    component.validateForm.patchValue({
      userName: 'operator@example.com',
      password: 'local-passphrase-2026',
      confirmPassword: 'different-passphrase-2026',
    });

    component.submitForm();

    expect(auth.register).not.toHaveBeenCalled();
    expect(notification.error).toHaveBeenCalledWith('Check your sign-up details', jasmine.any(String));
  });
});
