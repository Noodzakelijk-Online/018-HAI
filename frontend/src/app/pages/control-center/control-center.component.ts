import { Component, Inject, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { AUTOMATIONS_SERVICE_TOKEN } from '../../services/automations/automations.service.token'
import { IAutomationsService } from '../../services/automations.service.interface'
import {
  IAutomationDiagnostics,
  IAutomationHealthSummary,
  IAutomationModel,
} from '../../models/automation.model.interface'

@Component({
  selector: 'app-control-center',
  templateUrl: './control-center.component.html',
  styleUrls: ['./control-center.component.scss'],
})
export class ControlCenterComponent implements OnInit {
  automations: IAutomationModel[] = []
  summary?: IAutomationHealthSummary
  loading = false
  private checkingIds = new Set<string>()

  isDiagnosticsVisible = false
  diagnostics?: IAutomationDiagnostics
  diagnosticsLoading = false
  diagnosticsName = ''

  constructor(
    @Inject(AUTOMATIONS_SERVICE_TOKEN)
    private automationsService: IAutomationsService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh()
  }

  refresh(): void {
    this.loading = true
    this.automationsService.getAutomations().subscribe({
      next: (automations) => {
        this.automations = automations.sort((a, b) => a.position - b.position)
        this.loading = false
      },
      error: () => {
        this.loading = false
        this.notification.error('Error', 'Failed to load automations.')
      },
    })
    this.loadSummary()
  }

  loadSummary(): void {
    this.automationsService.getHealthSummary().subscribe({
      next: (summary) => (this.summary = summary),
      error: () =>
        this.notification.error('Error', 'Failed to load health summary.'),
    })
  }

  runHealthCheck(automation: IAutomationModel): void {
    if (!automation.id) {
      return
    }
    const id = automation.id
    this.checkingIds.add(id)
    this.automationsService.runHealthCheck(id).subscribe({
      next: (result) => {
        this.checkingIds.delete(id)
        automation.status = result.status
        automation.lastCheckedAt = result.checkedAt
        automation.averageLatencyMs = result.latencyMs
        automation.consecutiveFailures = result.consecutiveFailures
        if (result.status === 'healthy') {
          automation.lastSuccessAt = result.checkedAt
          automation.lastFailureReason = ''
        } else if (result.failureReason) {
          automation.lastFailureAt = result.checkedAt
          automation.lastFailureReason = result.failureReason
        }
        this.loadSummary()
        this.notification.success(
          'Health check',
          `${automation.name}: ${result.status}`
        )
      },
      error: () => {
        this.checkingIds.delete(id)
        this.notification.error(
          'Error',
          `Health check failed for ${automation.name}.`
        )
      },
    })
  }

  isChecking(automation: IAutomationModel): boolean {
    return !!automation.id && this.checkingIds.has(automation.id)
  }

  openDiagnostics(automation: IAutomationModel): void {
    if (!automation.id) {
      return
    }
    this.isDiagnosticsVisible = true
    this.diagnosticsLoading = true
    this.diagnostics = undefined
    this.diagnosticsName = automation.name
    this.automationsService.getDiagnostics(automation.id).subscribe({
      next: (diagnostics) => {
        this.diagnostics = diagnostics
        this.diagnosticsLoading = false
      },
      error: () => {
        this.diagnosticsLoading = false
        this.notification.error('Error', 'Failed to load diagnostics.')
      },
    })
  }

  closeDiagnostics(): void {
    this.isDiagnosticsVisible = false
    this.diagnostics = undefined
  }

  openTarget(automation: IAutomationModel): void {
    const url = this.targetUrl(automation)
    if (url) {
      window.open(url, '_blank', 'noopener')
    }
  }

  targetUrl(automation: IAutomationModel): string {
    return (
      automation.launchTarget ||
      automation.publicUrl ||
      automation.localUrl ||
      automation.healthCheckUrl ||
      ''
    )
  }

  isOpenable(automation: IAutomationModel): boolean {
    return /^https?:\/\//i.test(this.targetUrl(automation))
  }

  statusColor(status?: string): string {
    switch ((status || 'unknown').toLowerCase()) {
      case 'healthy':
        return 'green'
      case 'warning':
        return 'gold'
      case 'degraded':
        return 'orange'
      case 'broken':
        return 'red'
      default:
        return 'default'
    }
  }

  diagnosticsCheckKeys(): string[] {
    return this.diagnostics ? Object.keys(this.diagnostics.checks) : []
  }

  goHome(): void {
    this.router.navigate(['/home'])
  }
}
