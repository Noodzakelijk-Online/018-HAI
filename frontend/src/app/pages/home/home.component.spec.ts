import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ReactiveFormsModule } from '@angular/forms';
import { RouterTestingModule } from '@angular/router/testing';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { NzModalService } from 'ng-zorro-antd/modal';

import { HomeComponent } from './home.component';
import { AUTOMATIONS_SERVICE_TOKEN } from '../../services/automations/automations.service.token';
import { AUTH_SERVICE_TOKEN } from '../../services/auth/auth.service.token';
import { USER_SERVICE_TOKEN } from '../../services/user/user.service.token';

describe('HomeComponent', () => {
  let component: HomeComponent;
  let fixture: ComponentFixture<HomeComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({
      declarations: [HomeComponent],
      imports: [ReactiveFormsModule, RouterTestingModule],
      schemas: [NO_ERRORS_SCHEMA],
      providers: [
        { provide: NzNotificationService, useValue: {} },
        { provide: NzModalService, useValue: {} },
        { provide: AUTOMATIONS_SERVICE_TOKEN, useValue: {} },
        { provide: AUTH_SERVICE_TOKEN, useValue: {} },
        { provide: USER_SERVICE_TOKEN, useValue: {} },
      ],
    });
    fixture = TestBed.createComponent(HomeComponent);
    component = fixture.componentInstance;
    // Shallow creation smoke test: no detectChanges so ngOnInit's data loading
    // (which needs a full service mock) is not exercised here.
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
