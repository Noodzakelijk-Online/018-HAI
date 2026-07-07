import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ReactiveFormsModule } from '@angular/forms';
import { RouterTestingModule } from '@angular/router/testing';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { NzNotificationService } from 'ng-zorro-antd/notification';

import { LoginComponent } from './login.component';
import { AUTH_SERVICE_TOKEN } from '../../services/auth/auth.service.token';

describe('LoginComponent', () => {
  let component: LoginComponent;
  let fixture: ComponentFixture<LoginComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({
      declarations: [LoginComponent],
      imports: [ReactiveFormsModule, RouterTestingModule],
      schemas: [NO_ERRORS_SCHEMA],
      providers: [
        { provide: NzNotificationService, useValue: {} },
        { provide: AUTH_SERVICE_TOKEN, useValue: {} },
      ],
    });
    fixture = TestBed.createComponent(LoginComponent);
    component = fixture.componentInstance;
    // Shallow creation smoke test: no detectChanges so we assert DI/construction
    // without rendering the ng-zorro form template.
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
