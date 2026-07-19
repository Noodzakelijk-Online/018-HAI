import { Component, OnInit } from '@angular/core'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { BrainCatalogCollectionDisposition, IBrainCatalogEntry, IBrainCatalogOSSInsightDiscovery, IBrainCatalogOSSInsightDiscoveryReport, IBrainCatalogOSSInsightReview, IBrainCatalogResponse, IBrainCatalogUpstreamReview } from '../../models/brain-catalog.model.interface'
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
  reviewingDiscoveryRepository = ''
  verifyingDiscoveryRepository = ''
  revalidatingId = ''
  checkingOSSInsight = false
  discoveringOSSInsight = false
  upstreamReview?: IBrainCatalogUpstreamReview
  ossInsightReview?: IBrainCatalogOSSInsightReview
  ossInsightDiscovery?: IBrainCatalogOSSInsightDiscoveryReport
  discoveryReviews: Record<string, IBrainCatalogUpstreamReview> = {}

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

  revalidateOSSInsightCollections(): void {
    if (this.checkingOSSInsight) return
    this.checkingOSSInsight = true
    this.ossInsightReview = undefined
    this.service.revalidateOSSInsightCollections().subscribe({
      next: (review) => {
        this.checkingOSSInsight = false
        this.ossInsightReview = review
        const drift = (review.newCollections?.length ?? 0) + (review.missingExpected?.length ?? 0)
        this.notification.success('OSS Insight checked', drift ? 'Collection drift needs catalog review. No project was installed or activated.' : 'The collection snapshot still matches. No project was installed or activated.')
      },
      error: () => {
        this.checkingOSSInsight = false
        this.notification.error('OSS Insight check unavailable', 'HAI could not retrieve the public collection list. No catalog decision or runtime state changed.')
      },
    })
  }

  discoverOSSInsightRepositories(): void {
    if (this.discoveringOSSInsight) return
    this.discoveringOSSInsight = true
    this.ossInsightDiscovery = undefined
    this.service.discoverOSSInsightRepositories().subscribe({
      next: (report) => {
        this.discoveringOSSInsight = false
        this.ossInsightDiscovery = report
        this.discoveryReviews = {}
        this.notification.success('Candidate discovery complete', `${report.discoveries?.length ?? 0} unreviewed repositories were found. No catalog entry, credential, or runtime state changed.`)
      },
      error: () => {
        this.discoveringOSSInsight = false
        this.notification.error('Candidate discovery unavailable', 'HAI could not inspect the reviewed OSS Insight categories. No catalog decision or runtime state changed.')
      },
    })
  }

  verifyDiscovery(discovery: IBrainCatalogOSSInsightDiscovery): void {
    if (this.verifyingDiscoveryRepository) return
    this.verifyingDiscoveryRepository = discovery.repository
    this.service.revalidateOSSInsightDiscovery(discovery.repository).subscribe({
      next: (review) => {
        this.verifyingDiscoveryRepository = ''
        this.discoveryReviews = { ...this.discoveryReviews, [discovery.repository]: review }
        this.notification.success('Candidate metadata verified', `${discovery.repository} remains unconfigured. Review the metadata before creating a manual adapter review.`)
      },
      error: () => {
        this.verifyingDiscoveryRepository = ''
        this.notification.error('Candidate metadata unavailable', 'HAI could not verify this source-discovered repository. It was not added to the catalog or activated.')
      },
    })
  }

  queueDiscoveryReview(discovery: IBrainCatalogOSSInsightDiscovery): void {
    if (this.reviewingDiscoveryRepository) return
    this.reviewingDiscoveryRepository = discovery.repository
    this.pursuitService.create({
      title: `Screen ${discovery.repository} for a HAI adapter`,
      description: `Review discovered repository ${discovery.repository} from OSS Insight collection ${discovery.collection}. Source: ${discovery.sourceUrl}. This discovery record does not install, configure, credential, or execute the project.`,
      whyItMatters: discovery.rationale,
      projectKey: '018-HAI',
      domain: 'software',
      desiredOutcome: `A documented go/no-go decision for a narrow ${discovery.repository} adapter, including provenance, licence, maintenance, local deployment, data egress, health, audit, rollback, and approval boundaries.`,
      currentStateSummary: 'Repository was discovered from an already screened OSS Insight candidate category. It is not yet a HAI catalog profile and has no configured runtime.',
      status: 'waiting',
      priorityScore: 60,
      riskLevel: 'high',
      autonomyLevel: 'manual',
      sourceOfCreation: `ossinsight_discovery:${discovery.repository}`,
      nextRecommendedAction: 'Verify the fixed upstream repository metadata and licence, then define a local-only adapter and no-op validation plan before considering implementation.',
      completionDefinition: 'A human-approved acceptance or rejection record exists. Creating this pursuit must not install, enable, credential, or execute the discovered repository.',
    }).subscribe({
      next: (pursuit) => {
        this.reviewingDiscoveryRepository = ''
        this.notification.success('Discovery review created', `${discovery.repository} remains unconfigured. HAI created a manual review record.`)
        this.router.navigate(['/pursuits'], { queryParams: { selected: pursuit.id } })
      },
      error: () => {
        this.reviewingDiscoveryRepository = ''
        this.notification.error('Could not create discovery review', 'No repository was added to the catalog or activated. Check the local pursuit service and try again.')
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
