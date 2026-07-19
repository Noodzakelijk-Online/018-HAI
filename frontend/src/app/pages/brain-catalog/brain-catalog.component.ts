import { Component, OnInit } from '@angular/core'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { IBrainCatalogEntry, IBrainCatalogResponse } from '../../models/brain-catalog.model.interface'
import { BrainCatalogService } from '../../services/brain-catalog.service'

@Component({
  selector: 'app-brain-catalog',
  templateUrl: './brain-catalog.component.html',
  styleUrls: ['./brain-catalog.component.scss'],
})
export class BrainCatalogComponent implements OnInit {
  catalog?: IBrainCatalogResponse
  selected?: IBrainCatalogEntry
  loading = false

  constructor(private service: BrainCatalogService, private notification: NzNotificationService) {}

  ngOnInit(): void { this.refresh() }

  refresh(): void {
    this.loading = true
    this.service.overview().subscribe({
      next: (catalog) => {
        this.catalog = catalog
        this.selected = this.integrated[0] ?? catalog.entries[0]
        this.loading = false
      },
      error: () => {
        this.loading = false
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

  select(entry: IBrainCatalogEntry): void { this.selected = entry }

  statusColor(status: string): string {
    if (status === 'integrated_profile') return 'green'
    if (status === 'candidate') return 'blue'
    if (status === 'compatibility_only' || status === 'reference_only') return 'gold'
    return 'red'
  }

  statusLabel(status: string): string {
    return status.replace(/_/g, ' ')
  }
}
