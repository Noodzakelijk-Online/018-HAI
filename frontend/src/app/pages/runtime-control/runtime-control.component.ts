import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { forkJoin } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import {
  IBackgroundStatus,
  IReadiness,
} from '../../models/runtime-control.model.interface'
import { RuntimeControlService } from '../../services/runtime-control.service'

@Component({
  selector: 'app-runtime-control',
  templateUrl: './runtime-control.component.html',
  styleUrls: ['./runtime-control.component.scss'],
})
export class RuntimeControlComponent implements OnInit {
  status?: IBackgroundStatus
  readiness?: IReadiness
  loading = false
  busy = false

  readonly modes = ['autonomous_safe', 'approval_required', 'draft_only', 'read_only', 'paused']

  constructor(
    private service: RuntimeControlService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh()
  }

  refresh(): void {
    this.loading = true
    forkJoin({ status: this.service.status(), readiness: this.service.readiness() }).subscribe({
      next: ({ status, readiness }) => {
        this.status = status
        this.readiness = readiness
        this.loading = false
      },
      error: () => {
        this.loading = false
        this.notification.error('Error', 'Failed to load runtime control.')
      },
    })
  }

  pause(): void {
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
    this.service.setMode(mode).subscribe({
      next: () => {
        this.notification.success('Mode updated', `Autonomy mode set to ${mode}.`)
        this.refresh()
      },
      error: (err) => this.notification.error('Error', err?.error?.error ?? 'Mode change failed.'),
    })
  }

  verify(): void {
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
}
