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
  }

  private registerAccount(): void {
    const email = String(this.validateForm.value.userName || '').trim();
    const password = String(this.validateForm.value.password || '');
    const confirmation = String(this.validateForm.value.confirmPassword || '');
    const emailControl = this.validateForm.controls['userName'];
    emailControl.updateValueAndValidity();
    if (!email || emailControl.invalid || password.length < 12 || password !== confirmation) {
      this.validateForm.controls['userName'].markAsDirty();
      this.validateForm.controls['password'].markAsDirty();
      this.validateForm.controls['confirmPassword'].markAsDirty();
      this.notification.error('Check your sign-up details', 'Use a valid email, a password with at least 12 characters, and a matching confirmation.');
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
        this.notification.error('Sign-up failed', error?.error?.message || 'HAI could not create this account. Use a different email or try again later.');
      },
    });
  }

  showPasswordHelp(): void {
    this.notification.info(
      "Local account recovery",
      "Use the first-run admin credentials from .env.example or update the local IDP database/reset seed for this Windows install."
    );
  }
}
