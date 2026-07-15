import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { forkJoin } from 'rxjs'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import {
  IHardwareResponse,
  IModelIntelligenceOverview,
  IModelProfile,
  IOperationBudget,
  IPowerPolicy,
} from '../../models/model-intelligence.model.interface'
import { ModelIntelligenceService } from '../../services/model-intelligence.service'

@Component({
  selector: 'app-model-intelligence',
  templateUrl: './model-intelligence.component.html',
  styleUrls: ['./model-intelligence.component.scss'],
})
export class ModelIntelligenceComponent implements OnInit {
  overview?: IModelIntelligenceOverview
  profiles: IModelProfile[] = []
  budgets?: IOperationBudget
  hardware?: IHardwareResponse
  power?: IPowerPolicy

  loading = false
  benchmarking: Record<string, boolean> = {}

  constructor(
    private service: ModelIntelligenceService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh()
  }

  refresh(): void {
    this.loading = true
    forkJoin({
      overview: this.service.overview(),
      profiles: this.service.profiles(),
      budgets: this.service.tokenBudgets(),
      hardware: this.service.hardware(),
      power: this.service.powerPolicy(),
    }).subscribe({
      next: ({ overview, profiles, budgets, hardware, power }) => {
        this.overview = overview
        this.profiles = profiles.profiles ?? []
        this.budgets = budgets
        this.hardware = hardware
        this.power = power
        this.loading = false
      },
      error: () => {
        this.loading = false
        this.notification.error('Error', 'Failed to load model intelligence.')
      },
    })
  }

  benchmark(p: IModelProfile): void {
    const key = p.providerId + '/' + p.modelId
    this.benchmarking[key] = true
    this.service.benchmark(p.providerId, p.modelId).subscribe({
      next: (res) => {
        this.benchmarking[key] = false
        if (res.ok) {
          this.notification.success('Benchmarked', `${key}: ${res.tokensPerSecond.toFixed(0)} tok/s`)
        } else {
          this.notification.warning('Not benchmarked', res.detail ?? `${key} is not usable`)
        }
        this.refresh()
      },
      error: () => {
        this.benchmarking[key] = false
        this.notification.error('Error', 'Benchmark failed.')
      },
    })
  }

  detectHardware(): void {
    this.service.detectHardware().subscribe({
      next: (hw) => {
        this.hardware = hw
        this.notification.success('Detected', `Serving stack: ${hw.selectedServingStack}`)
      },
      error: () => this.notification.error('Error', 'Hardware detect failed.'),
    })
  }

  statusColor(status: string): string {
    switch (status) {
      case 'active':
        return 'green'
      case 'configured':
        return 'blue'
      case 'not_configured':
        return 'default'
      case 'unavailable':
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
