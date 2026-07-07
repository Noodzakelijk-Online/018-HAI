import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ReactiveFormsModule } from '@angular/forms';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { NzNotificationService } from 'ng-zorro-antd/notification';

import { AutomationsFormComponent } from './automations-form.component';

describe('AutomationsFormComponent', () => {
  let component: AutomationsFormComponent;
  let fixture: ComponentFixture<AutomationsFormComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({
      declarations: [AutomationsFormComponent],
      imports: [ReactiveFormsModule],
      schemas: [NO_ERRORS_SCHEMA],
      providers: [{ provide: NzNotificationService, useValue: {} }],
    });
    fixture = TestBed.createComponent(AutomationsFormComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
