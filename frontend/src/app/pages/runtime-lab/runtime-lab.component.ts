import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { IRuntimeSummary } from '../../models/runtime-lab.model.interface'
import { RuntimeLabService } from '../../services/runtime-lab.service'

@Component({
  selector: 'app-runtime-lab',
  templateUrl: './runtime-lab.component.html',
  styleUrls: ['./runtime-lab.component.scss'],
})
export class RuntimeLabComponent implements OnInit {
  runtimes: IRuntimeSummary[] = []
  loading = false
  busy: Record<string, boolean> = {}

  constructor(
    private service: RuntimeLabService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh()
  }

  refresh(): void {
    this.loading = true
    this.service.overview().subscribe({
      next: (res) => {
        this.runtimes = res.runtimes ?? []
        this.loading = false
      },
      error: () => {
        this.loading = false
        this.notification.error('Error', 'Failed to load the runtime lab.')
      },
    })
  }

  probe(r: IRuntimeSummary): void {
    this.busy[r.info.id] = true
    this.service.probe(r.info.id).subscribe({
      next: (res) => {
        this.busy[r.info.id] = false
        this.notification.info('Probe', `${r.info.displayName}: ${res.status}`)
        this.refresh()
      },
      error: () => {
        this.busy[r.info.id] = false
        this.notification.error('Error', 'Probe failed.')
      },
    })
  }

  selfTest(r: IRuntimeSummary): void {
    this.busy[r.info.id] = true
    this.service.selfTest(r.info.id).subscribe({
      next: (attempt) => {
        this.busy[r.info.id] = false
        if (attempt.status === 'succeeded') {
          this.notification.success('Self-test passed', `${r.info.displayName} verified through the ledger.`)
        } else if (attempt.status === 'setup_required') {
          this.notification.warning('Setup required', `${r.info.displayName} is not configured — no fake execution.`)
        } else {
          this.notification.warning('Self-test', `${r.info.displayName}: ${attempt.status}`)
        }
        this.refresh()
      },
      error: () => {
        this.busy[r.info.id] = false
        this.notification.error('Error', 'Self-test failed.')
      },
    })
  }

  statusColor(status: string): string {
    switch (status) {
      case 'ready':
        return 'green'
      case 'configured':
        return 'blue'
      case 'not_configured':
        return 'default'
      case 'blocked':
        return 'gold'
      case 'unavailable':
      case 'failed':
        return 'red'
      default:
        return 'default'
    }
  }

  attemptColor(status?: string): string {
    switch (status) {
      case 'succeeded':
        return 'green'
      case 'setup_required':
        return 'default'
      case 'inconclusive':
        return 'gold'
      case 'failed':
      case 'blocked':
        return 'red'
      default:
        return 'default'
    }
  }

  goBack(): void {
    this.router.navigate(['/control-center'])
  }
}
