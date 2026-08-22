import { ComponentFixture, TestBed } from '@angular/core/testing'
import { RouterTestingModule } from '@angular/router/testing'
import { NoopAnimationsModule } from '@angular/platform-browser/animations'
import {
  ArrowLeftOutline,
  CheckCircleOutline,
  ReloadOutline,
  StopOutline,
} from '@ant-design/icons-angular/icons'
import { of, throwError } from 'rxjs'
import { NZ_ICONS } from 'ng-zorro-antd/icon'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { IAuthSession } from '../../models/auth-session.model.interface'
import {
  IBackgroundStatus,
  IReadiness,
} from '../../models/runtime-control.model.interface'
import { AuthSessionService } from '../../services/auth-session.service'
import { RuntimeControlService } from '../../services/runtime-control.service'
import { RuntimeControlComponent } from './runtime-control.component'
import { RuntimeControlModule } from './runtime-control.module'

describe('RuntimeControlComponent role boundaries', () => {
  let fixture: ComponentFixture<RuntimeControlComponent>
  let component: RuntimeControlComponent
  let runtimeService: jasmine.SpyObj<RuntimeControlService>
  let authSessionService: jasmine.SpyObj<AuthSessionService>
  let notification: jasmine.SpyObj<NzNotificationService>

  const ownerSession: IAuthSession = {
    authenticated: true,
    subject: 'owner-1',
    role: 'owner',
    permissions: {
      canRead: true,
      canOperate: true,
      canApprove: true,
      canAdminister: true,
    },
  }

  const operatorSession: IAuthSession = {
    authenticated: true,
    subject: 'operator-1',
    role: 'operator',
    permissions: {
      canRead: true,
      canOperate: true,
      canApprove: true,
      canAdminister: false,
    },
  }

  const viewerSession: IAuthSession = {
    authenticated: true,
    subject: 'viewer-1',
    role: 'viewer',
    permissions: {
      canRead: true,
      canOperate: false,
      canApprove: false,
      canAdminister: false,
    },
  }

  const activeStatus: IBackgroundStatus = {
    mode: 'approval_required',
    storedMode: 'approval_required',
    emergencyStop: {
      engaged: false,
      updatedAt: '2026-07-30T12:00:00Z',
    },
    backgroundProcessingActive: true,
    docker: {
      cliAvailable: true,
      daemonRunning: true,
      required: false,
      detail: 'available',
    },
    completedOperations: 4,
    awaitingApproval: 2,
  }

  const readiness: IReadiness = {
    operatingSystem: 'windows',
    isWindows: true,
    overallReady: true,
    targetMachineVerificationPending: false,
    backgroundMode: 'approval_required',
    emergencyStop: activeStatus.emergencyStop,
    docker: activeStatus.docker,
    gates: [],
  }

  beforeEach(async () => {
    runtimeService = jasmine.createSpyObj<RuntimeControlService>('RuntimeControlService', [
      'status',
      'readiness',
      'pause',
      'resume',
	  'requestResumeApproval',
	  'approveAndResume',
      'setMode',
      'verifyEmergencyStop',
      'recover',
    ])
    authSessionService = jasmine.createSpyObj<AuthSessionService>('AuthSessionService', [
      'session',
    ])
    notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', [
      'success',
      'warning',
      'error',
	  'info',
    ])

    runtimeService.status.and.returnValue(of(activeStatus))
    runtimeService.readiness.and.returnValue(of(readiness))
    runtimeService.pause.and.returnValue(of({ emergencyStop: activeStatus.emergencyStop }))
    runtimeService.resume.and.returnValue(of({ emergencyStop: activeStatus.emergencyStop }))
	    runtimeService.requestResumeApproval.and.returnValue(of({
	      reviewItemId: 'review-1',
	      approvalSourceId: 'opscontrol-review:review-1',
	      approvalBindingDigest: 'a'.repeat(64),
	    }))
	    runtimeService.approveAndResume.and.returnValue(of({ emergencyStop: activeStatus.emergencyStop }))
    runtimeService.setMode.and.returnValue(of({ mode: 'read_only' }))
    runtimeService.verifyEmergencyStop.and.returnValue(of({
      engagedDuringTest: true,
      operationsProcessedDuringStop: 0,
      halted: true,
      restoredEngagedState: true,
      detail: 'Emergency stop halted processing.',
    }))
    runtimeService.recover.and.returnValue(of({
      scannedRunning: 1,
      scannedVerifying: 0,
      recovered: 1,
      ranAt: '2026-07-30T12:00:00Z',
    }))

    await TestBed.configureTestingModule({
      imports: [
        RuntimeControlModule,
        RouterTestingModule,
        NoopAnimationsModule,
      ],
      providers: [
        { provide: RuntimeControlService, useValue: runtimeService },
        { provide: AuthSessionService, useValue: authSessionService },
        { provide: NzNotificationService, useValue: notification },
        {
          provide: NZ_ICONS,
          useValue: [
            ArrowLeftOutline,
            CheckCircleOutline,
            ReloadOutline,
            StopOutline,
          ],
        },
      ],
    }).compileComponents()
  })

  it('requires a durable owner review before resuming and then consumes it', () => {
    authSessionService.session.and.returnValue(of(ownerSession))
    runtimeService.status.and.returnValue(of({
      ...activeStatus,
      emergencyStop: {
        ...activeStatus.emergencyStop,
        engaged: true,
      },
      backgroundProcessingActive: false,
    }))
    createComponent()

    expect(text()).toContain('Owner authority')
    expect(button('Prepare resume review').disabled).toBeFalse()

    button('Prepare resume review').click()
    fixture.detectChanges()
    expect(text()).toContain('Review prepared for the current emergency stop')
    expect(button('Confirm and resume').disabled).toBeFalse()
    button('Confirm and resume').click()
    component.setMode('read_only')

    expect(runtimeService.requestResumeApproval).toHaveBeenCalled()
    expect(runtimeService.approveAndResume).toHaveBeenCalledWith('review-1')
    expect(runtimeService.resume).not.toHaveBeenCalled()
    expect(runtimeService.setMode).toHaveBeenCalledWith('read_only')
    expect(component.canAdministerRuntime).toBeTrue()
  })

  it('keeps emergency stop, recovery, and verification available to an operator', () => {
    authSessionService.session.and.returnValue(of(operatorSession))
    createComponent()

    expect(text()).toContain('Operator authority')
    expect(button('Engage emergency stop').disabled).toBeFalse()
    expect(button('Verify emergency stop').disabled).toBeFalse()
    expect(button('Run recovery').disabled).toBeFalse()
    expect(radioInputs().every((input) => input.disabled)).toBeTrue()

    button('Engage emergency stop').click()
    component.verify()
    component.recover()
    component.setMode('read_only')

    expect(runtimeService.pause).toHaveBeenCalled()
    expect(runtimeService.verifyEmergencyStop).toHaveBeenCalled()
    expect(runtimeService.recover).toHaveBeenCalled()
    expect(runtimeService.setMode).not.toHaveBeenCalled()

    component.status = {
      ...activeStatus,
      emergencyStop: {
        ...activeStatus.emergencyStop,
        engaged: true,
      },
    }
    fixture.detectChanges()

    expect(button('Prepare resume review').disabled).toBeTrue()
    component.resume()
    expect(runtimeService.resume).not.toHaveBeenCalled()
    expect(text()).toContain('Only the owner can resume processing')
  })

  it('renders a viewer as read-only and blocks every protected interaction', () => {
    authSessionService.session.and.returnValue(of(viewerSession))
    createComponent()

    expect(text()).toContain('Read-only authority')
    expect(button('Engage emergency stop').disabled).toBeTrue()
    expect(button('Verify emergency stop').disabled).toBeTrue()
    expect(button('Run recovery').disabled).toBeTrue()
    expect(radioInputs().every((input) => input.disabled)).toBeTrue()

    component.pause()
    component.resume()
    component.verify()
    component.recover()
    component.setMode('paused')

    expect(runtimeService.pause).not.toHaveBeenCalled()
    expect(runtimeService.resume).not.toHaveBeenCalled()
    expect(runtimeService.verifyEmergencyStop).not.toHaveBeenCalled()
    expect(runtimeService.recover).not.toHaveBeenCalled()
    expect(runtimeService.setMode).not.toHaveBeenCalled()
    expect(notification.warning).toHaveBeenCalled()
  })

  it('fails closed when the signed session is unavailable', () => {
    authSessionService.session.and.returnValue(
      throwError(() => new Error('session unavailable'))
    )
    createComponent()

    expect(text()).toContain('Authority unavailable')
    expect(text()).toContain('Runtime controls locked')
    expect(runtimeService.status).not.toHaveBeenCalled()
    expect(runtimeService.readiness).not.toHaveBeenCalled()
    expect(component.canReadRuntime).toBeFalse()
    expect(fixture.nativeElement.querySelector('.rc__control')).toBeNull()
  })

  function createComponent(): void {
    fixture = TestBed.createComponent(RuntimeControlComponent)
    component = fixture.componentInstance
    fixture.detectChanges()
  }

  function text(): string {
    return (fixture.nativeElement as HTMLElement).textContent ?? ''
  }

  function button(label: string): HTMLButtonElement {
    const match = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('button')
    ).find((candidate) => candidate.textContent?.includes(label))
    if (!match) {
      throw new Error(`Button not found: ${label}`)
    }
    return match
  }

  function radioInputs(): HTMLInputElement[] {
    return Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(
        'input[type="radio"]'
      )
    )
  }
})
