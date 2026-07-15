import { Component, Inject, OnInit } from "@angular/core";
import { FormBuilder, FormGroup, Validators } from "@angular/forms";
import { NzNotificationService } from "ng-zorro-antd/notification";
import { AUTH_SERVICE_TOKEN } from "../../services/auth/auth.service.token";
import { IAuthService } from "../../services/auth.service.interface";
import { Router } from "@angular/router";

@Component({
  selector: "app-login",
  templateUrl: "./login.component.html",
  styleUrls: ["./login.component.scss"],
})
export class LoginComponent implements OnInit {
  readonly minimumRegistrationPasswordLength = 12;
  hidePassword: boolean = true;
  registrationMode = false;
  registering = false;
  validateForm: FormGroup = this.fb.group({});

  constructor(
    private fb: FormBuilder,
    private notification: NzNotificationService,
    private router: Router,
    @Inject(AUTH_SERVICE_TOKEN) private authService: IAuthService
  ) {}

  ngOnInit(): void {
    this.onInitForm();
  }

  onInitForm() {
    this.validateForm = this.fb.group({
      userName: [
        "",
        {
          validators: [Validators.required, Validators.email],
        },
      ],
      password: ["", { updateOn: "submit", validators: [Validators.required] }],
      confirmPassword: [""],
      remember: [true],
    });
  }

  // Full-page redirect into the IDP's Google OAuth flow. It is not an XHR: the
  // browser must follow Google's redirects and land back on the app with the
  // session cookies set by the callback.
  loginWithGoogle(): void {
    window.location.href = '/api/v1/auth/google/login';
  }

  submitForm(): void {
    if (this.registrationMode) {
      this.registerAccount();
      return;
    }
    if (!this.validateForm.valid) {
      for (const i in this.validateForm.controls) {
        this.validateForm.controls[i].markAsDirty();
        this.validateForm.controls[i].updateValueAndValidity();
      }
      return;
    }

    this.authService
      .login(this.validateForm.value.userName, this.validateForm.value.password)
      .subscribe({
        next: () => {
          this.router.navigate(["/control-center"]);
        },
        error: (error) => {
          if (error.status === 401) {
            this.notification.create(
              "error",
              "Login Failed",
              "Incorrect username or password."
            );
          } else {
            this.notification.create(
              "error",
              "Network Error",
              "An unexpected error occurred. Please try again later."
            );
          }
        },
      });
  }

  toggleRegistration(): void {
    this.registrationMode = !this.registrationMode;
    this.registering = false;
    this.validateForm.controls['password'].reset();
    this.validateForm.controls['confirmPassword'].reset();
    this.validateForm.controls['password'].markAsPristine();
    this.validateForm.controls['confirmPassword'].markAsPristine();
    this.validateForm.controls['password'].markAsUntouched();
    this.validateForm.controls['confirmPassword'].markAsUntouched();
  }

  passwordErrorTip(): string {
    const passwordControl = this.validateForm.controls['password'];
    if (this.registrationMode && passwordControl.hasError('minLength')) {
      return `Use at least ${this.minimumRegistrationPasswordLength} characters.`;
    }
    return 'Please input your password!';
  }

  confirmationErrorTip(): string {
    const confirmationControl = this.validateForm.controls['confirmPassword'];
    if (confirmationControl.hasError('mismatch')) {
      return 'Passwords do not match.';
    }
    return 'Please confirm your password.';
  }

  emailErrorTip(): string {
    if (this.registrationMode && this.validateForm.controls['userName'].hasError('accountExists')) {
      return 'This email is already registered. Log in instead.';
    }
    return 'Please input a valid email!';
  }

  private registerAccount(): void {
    const email = String(this.validateForm.value.userName || '').trim();
    const password = String(this.validateForm.value.password || '');
    const confirmation = String(this.validateForm.value.confirmPassword || '');
    const emailControl = this.validateForm.controls['userName'];
    const passwordControl = this.validateForm.controls['password'];
    const confirmationControl = this.validateForm.controls['confirmPassword'];
    emailControl.updateValueAndValidity();

    this.clearValidationError(passwordControl, 'minLength');
    this.clearValidationError(confirmationControl, 'mismatch');
    if (password.length < this.minimumRegistrationPasswordLength) {
      passwordControl.setErrors({ ...passwordControl.errors, minLength: true });
    }
    if (!confirmation) {
      confirmationControl.setErrors({ ...confirmationControl.errors, required: true });
    } else if (password !== confirmation) {
      confirmationControl.setErrors({ ...confirmationControl.errors, mismatch: true });
    } else {
      this.clearValidationError(confirmationControl, 'required');
    }

    if (!email || emailControl.invalid || passwordControl.invalid || confirmationControl.invalid) {
      [emailControl, passwordControl, confirmationControl].forEach((control) => {
        control.markAsDirty();
        control.markAsTouched();
      });
      this.notification.error('Check your sign-up details', this.signupValidationSummary());
      return;
    }
    this.registering = true;
    this.authService.register(email, password).subscribe({
      next: () => {
        this.registering = false;
        this.registrationMode = false;
        this.validateForm.patchValue({ userName: email, password: '', confirmPassword: '' });
        this.notification.success('Account created', 'Your local operator account is ready. Sign in to continue.');
      },
      error: (error) => {
        this.registering = false;
        if (error?.status === 409) {
          emailControl.setErrors({ ...emailControl.errors, accountExists: true });
          emailControl.markAsDirty();
          emailControl.markAsTouched();
          this.notification.error('Account already exists', 'This email is already registered. Log in instead or use password recovery.');
          return;
        }
        this.notification.error('Sign-up failed', error?.error?.message || 'HAI could not create this account. Use a different email or try again later.');
      },
    });
  }

  private clearValidationError(control: any, errorName: string): void {
    if (!control.hasError(errorName)) {
      return;
    }
    const errors = { ...control.errors };
    delete errors[errorName];
    control.setErrors(Object.keys(errors).length ? errors : null);
  }

  private signupValidationSummary(): string {
    const emailControl = this.validateForm.controls['userName'];
    const passwordControl = this.validateForm.controls['password'];
    const confirmationControl = this.validateForm.controls['confirmPassword'];
    if (emailControl.invalid) {
      return 'Enter a valid email address.';
    }
    if (passwordControl.hasError('minLength')) {
      return `Your password must contain at least ${this.minimumRegistrationPasswordLength} characters.`;
    }
    if (confirmationControl.hasError('mismatch')) {
      return 'The confirmation password does not match.';
    }
    return 'Confirm your password to create the account.';
  }

  showPasswordHelp(): void {
    this.notification.info(
      "Local account recovery",
      "Use the first-run admin credentials from .env.example or update the local IDP database/reset seed for this Windows install."
    );
  }
}
