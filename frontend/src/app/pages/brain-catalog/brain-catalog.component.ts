import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { BrainCatalogCollectionDisposition, IBrainCatalogEntry, IBrainCatalogResponse, IBrainCatalogUpstreamReview } from '../../models/brain-catalog.model.interface'
import { BrainCatalogService } from '../../services/brain-catalog.service'
import { PursuitService } from '../../services/pursuit.service'

@Component({
  selector: 'app-brain-catalog',
  templateUrl: './brain-catalog.component.html',
  styleUrls: ['./brain-catalog.component.scss'],
})
export class BrainCatalogComponent implements OnInit {
  catalog?: IBrainCatalogResponse
  selected?: IBrainCatalogEntry
  loading = false
  loadFailed = false
  reviewingCandidateId = ''
  revalidatingId = ''
  upstreamReview?: IBrainCatalogUpstreamReview

  constructor(
    private service: BrainCatalogService,
    private pursuitService: PursuitService,
    private notification: NzNotificationService,
    private router: Router,
  ) {}

  ngOnInit(): void { this.refresh() }

  refresh(): void {
    this.loading = true
    this.loadFailed = false
    this.service.overview().subscribe({
      next: (catalog) => {
        this.catalog = catalog
        this.selected = this.integrated[0] ?? catalog.entries[0]
        this.loading = false
      },
      error: () => {
        this.loading = false
        this.loadFailed = true
        this.notification.error('Catalog unavailable', 'HAI could not load the reviewed capability catalog.')
      },
    })
  }

  get integrated(): IBrainCatalogEntry[] {
    return (this.catalog?.entries ?? []).filter((entry) => entry.status === 'integrated_profile')
  }

  get candidates(): IBrainCatalogEntry[] {
    return (this.catalog?.entries ?? []).filter((entry) =>
      entry.status === 'candidate' || entry.status === 'compatibility_only'
    )
  }

  get held(): IBrainCatalogEntry[] {
    return (this.catalog?.entries ?? []).filter((entry) =>
      entry.status === 'reference_only' || entry.status === 'license_review' || entry.status === 'excluded'
    )
  }

  select(entry: IBrainCatalogEntry): void {
    this.selected = entry
    this.upstreamReview = undefined
  }

  revalidate(entry: IBrainCatalogEntry): void {
    if (this.revalidatingId) return
    this.revalidatingId = entry.id
    this.upstreamReview = undefined
    this.service.revalidate(entry.id).subscribe({
      next: (review) => {
        this.revalidatingId = ''
        this.upstreamReview = review
        this.notification.success('Upstream rechecked', `${entry.name} was checked without changing its HAI activation state.`)
      },
      error: () => {
        this.revalidatingId = ''
        this.notification.error('Upstream recheck unavailable', 'HAI could not retrieve public GitHub metadata. No catalog decision or runtime state changed.')
      },
    })
  }

  canStartReview(entry: IBrainCatalogEntry): boolean {
    return entry.status === 'candidate' || entry.status === 'compatibility_only'
  }

  startAdapterReview(entry: IBrainCatalogEntry): void {
    if (!this.canStartReview(entry) || this.reviewingCandidateId) return
    this.reviewingCandidateId = entry.id
    const riskLevel = entry.requiresApproval ? 'high' : 'medium'
    this.pursuitService.create({
      title: `Review ${entry.name} adapter boundary`,
      description: `Review the HAI adapter proposal for ${entry.name}. Upstream: ${entry.upstreamUrl}. Discovery source: ${entry.sourceCatalogUrl}. HAI must not install, enable, or execute ${entry.name} during this review.`,
      whyItMatters: entry.rationale,
      projectKey: '018-HAI',
      domain: 'software',
      desiredOutcome: `A documented go/no-go decision for a narrow ${entry.name} adapter, including local configuration, health checks, audit records, rollback, and approval boundaries.`,
      currentStateSummary: `Catalog candidate recorded as ${entry.status.replace(/_/g, ' ')}. No HAI adapter is configured or live.`,
      status: 'waiting',
      priorityScore: entry.requiresApproval ? 70 : 55,
      riskLevel,
      autonomyLevel: 'manual',
      sourceOfCreation: `brain_catalog:${entry.id}`,
      nextRecommendedAction: 'Review license, deployment model, workspace and network boundaries, allowed tools, health probe, audit trail, rollback, and the no-op validation path before implementation.',
      completionDefinition: 'A human-approved adapter design and validation plan exists, or the candidate is explicitly rejected. No third-party software is installed or activated by creating this pursuit.',
    }).subscribe({
      next: (pursuit) => {
        this.reviewingCandidateId = ''
        this.notification.success('Adapter review created', `${entry.name} remains disabled. HAI created a review record instead of activating the project.`)
        this.router.navigate(['/pursuits'], { queryParams: { selected: pursuit.id } })
      },
      error: () => {
        this.reviewingCandidateId = ''
        this.notification.error('Could not create adapter review', 'No project was installed, configured, or activated. Try again after checking the local pursuit service.')
      },
    })
  }

  statusColor(status: string): string {
    if (status === 'integrated_profile') return 'green'
    if (status === 'candidate') return 'blue'
    if (status === 'compatibility_only' || status === 'reference_only') return 'gold'
    return 'red'
  }

  statusLabel(status: string): string {
    return status.replace(/_/g, ' ')
  }

  collectionDispositionColor(disposition: BrainCatalogCollectionDisposition): string {
    if (disposition === 'represented_in_catalog') return 'green'
    if (disposition === 'review_candidate') return 'blue'
    if (disposition === 'reference_only') return 'gold'
    return 'default'
  }

  collectionDispositionLabel(disposition: BrainCatalogCollectionDisposition): string {
    if (disposition === 'represented_in_catalog') return 'represented'
    if (disposition === 'review_candidate') return 'review candidate'
    if (disposition === 'reference_only') return 'reference only'
    return 'not adopted'
  }
}
