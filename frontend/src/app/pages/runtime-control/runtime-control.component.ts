import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { catchError, finalize, forkJoin, of, switchMap } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import {
  AuthActorRole,
  IAuthSession,
} from '../../models/auth-session.model.interface'
import {
  IBackgroundStatus,
  IReadiness,
} from '../../models/runtime-control.model.interface'
import { AuthSessionService } from '../../services/auth-session.service'
import { RuntimeControlService } from '../../services/runtime-control.service'

@Component({
  standalone: false,
  selector: 'app-runtime-control',
  templateUrl: './runtime-control.component.html',
  styleUrls: ['./runtime-control.component.scss'],
})
export class RuntimeControlComponent implements OnInit {
  status?: IBackgroundStatus
  readiness?: IReadiness
  authSession?: IAuthSession
  loading = false
  busy = false
  sessionUnavailable = false
  runtimeLoadError = ''

  readonly modes = ['autonomous_safe', 'approval_required', 'draft_only', 'read_only', 'paused']

  constructor(
    private service: RuntimeControlService,
    private authSessionService: AuthSessionService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh()
  }

  refresh(): void {
    this.loading = true
    this.sessionUnavailable = false
    this.runtimeLoadError = ''
    this.authSession = undefined
    this.status = undefined
    this.readiness = undefined

    this.authSessionService.session().pipe(
      catchError(() => {
        this.sessionUnavailable = true
        return of(undefined)
      }),
      switchMap((session) => {
        this.authSession = session
        if (!this.canReadRuntime) {
          return of({ status: undefined, readiness: undefined })
        }

        return forkJoin({
          status: this.service.status(),
          readiness: this.service.readiness(),
        }).pipe(
          catchError(() => {
            this.runtimeLoadError =
              'Runtime state could not be loaded. No controls were executed.'
            return of({ status: undefined, readiness: undefined })
          })
        )
      }),
      finalize(() => (this.loading = false))
    ).subscribe(({ status, readiness }) => {
      this.status = status
      this.readiness = readiness
    })
  }

  pause(): void {
    if (!this.requirePermission(
      this.canApproveRuntimeActions,
      'Emergency stop',
      this.approvalControlExplanation
    )) {
      return
    }
    this.busy = true
    this.service.pause('operator engaged emergency stop').subscribe({
      next: () => {
        this.busy = false
        this.notification.warning('Emergency stop engaged', 'Background processing is halted.')
        this.refresh()
      },
      error: () => {
        this.busy = false
        this.notification.error('Error', 'Pause failed.')
      },
    })
  }

  resume(): void {
    if (!this.requirePermission(
      this.canAdministerRuntime,
      'Resume',
      this.ownerControlExplanation
    )) {
      return
    }
    this.busy = true
    this.service.resume().subscribe({
      next: () => {
        this.busy = false
        this.notification.success('Resumed', 'Background processing re-enabled.')
        this.refresh()
      },
      error: () => {
        this.busy = false
        this.notification.error('Error', 'Resume failed.')
      },
    })
  }

  setMode(mode: string): void {
    if (!this.requirePermission(
      this.canAdministerRuntime,
      'Autonomy mode change',
      this.ownerControlExplanation
    )) {
      return
    }
    this.service.setMode(mode).subscribe({
      next: () => {
        this.notification.success('Mode updated', `Autonomy mode set to ${mode}.`)
        this.refresh()
      },
      error: (err) => this.notification.error('Error', err?.error?.error ?? 'Mode change failed.'),
    })
  }

  verify(): void {
    if (!this.requirePermission(
      this.canApproveRuntimeActions,
      'Emergency-stop verification',
      this.approvalControlExplanation
    )) {
      return
    }
    this.busy = true
    this.service.verifyEmergencyStop().subscribe({
      next: (v) => {
        this.busy = false
        if (v.halted) {
          this.notification.success('Emergency stop verified', v.detail)
        } else {
          this.notification.error('EMERGENCY STOP FAILED', v.detail)
        }
        this.refresh()
      },
      error: () => {
        this.busy = false
        this.notification.error('Error', 'Verification failed.')
      },
    })
  }

  recover(): void {
    if (!this.requirePermission(
      this.canApproveRuntimeActions,
      'Runtime recovery',
      this.approvalControlExplanation
    )) {
      return
    }
    this.busy = true
    this.service.recover().subscribe({
      next: (r) => {
        this.busy = false
        this.notification.success('Recovery complete', `${r.recovered} operations recovered.`)
        this.refresh()
      },
      error: () => {
        this.busy = false
        this.notification.error('Error', 'Recovery failed.')
      },
    })
  }

  get actorRole(): AuthActorRole {
    return this.authSession?.role ?? 'unknown'
  }

  get canReadRuntime(): boolean {
    return Boolean(
      this.authSession?.authenticated &&
      this.authSession.permissions.canRead
    )
  }

  get canApproveRuntimeActions(): boolean {
    return Boolean(
      this.authSession?.authenticated &&
      this.authSession.permissions.canApprove
    )
  }

  get canAdministerRuntime(): boolean {
    return Boolean(
      this.authSession?.authenticated &&
      this.authSession.permissions.canAdminister
    )
  }

  get authorityTitle(): string {
    switch (this.actorRole) {
      case 'owner':
        return 'Owner authority'
      case 'operator':
        return 'Operator authority'
      case 'viewer':
        return 'Read-only authority'
      default:
        return 'Authority unavailable'
    }
  }

  get authorityDescription(): string {
    switch (this.actorRole) {
      case 'owner':
        return 'Emergency stop, recovery, verification, resume, and autonomy controls are available.'
      case 'operator':
        return 'Emergency stop, recovery, and verification are available. Resume and autonomy changes require the owner.'
      case 'viewer':
        return 'Runtime state is read-only. Operational and owner controls are disabled.'
      default:
        return 'HAI could not verify signed session permissions. All runtime changes are disabled.'
    }
  }

  get authorityTagColor(): string {
    switch (this.actorRole) {
      case 'owner':
        return 'blue'
      case 'operator':
        return 'green'
      case 'viewer':
        return 'default'
      default:
        return 'gold'
    }
  }

  get ownerControlExplanation(): string {
    if (this.actorRole === 'operator') {
      return 'Only the owner can resume processing or change the autonomy mode.'
    }
    if (this.actorRole === 'viewer') {
      return 'Viewer sessions are read-only. Only the owner can change runtime policy.'
    }
    return 'Owner controls are disabled because HAI could not verify owner authority.'
  }

  get approvalControlExplanation(): string {
    if (this.actorRole === 'viewer') {
      return 'Viewer sessions are read-only. An operator or owner is required.'
    }
    return 'This control is disabled because HAI could not verify approval authority.'
  }

  gateColor(status: string): string {
    switch (status) {
      case 'pass':
        return 'green'
      case 'warn':
        return 'gold'
      case 'fail':
        return 'red'
      case 'pending':
        return 'blue'
      case 'not_applicable':
        return 'default'
      default:
        return 'default'
    }
  }

  goBack(): void {
    this.router.navigate(['/control-center'])
  }

  private requirePermission(
    permitted: boolean,
    action: string,
    explanation: string
  ): boolean {
    if (permitted && !this.busy) {
      return true
    }
    if (!permitted) {
      this.notification.warning(`${action} unavailable`, explanation)
    }
    return false
  }
}
