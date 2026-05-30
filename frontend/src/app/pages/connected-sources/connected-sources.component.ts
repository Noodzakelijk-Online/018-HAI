import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  IConnectedSource,
  ISourceAuditLog,
  ISourceConnector,
  ISourceExtraction,
  ISourceSearchResult,
} from '../../models/connected-source.model.interface';
import { CONNECTED_SOURCE_SERVICE_TOKEN } from '../../services/connected-source/connected-source.service.token';
import { IConnectedSourceService } from '../../services/connected-source.service.interface';

@Component({
  selector: 'app-connected-sources',
  templateUrl: './connected-sources.component.html',
  styleUrls: ['./connected-sources.component.scss'],
})
export class ConnectedSourcesComponent implements OnInit {
  connectors: ISourceConnector[] = [];
  sources: IConnectedSource[] = [];
  extractions: ISourceExtraction[] = [];
  auditLogs: ISourceAuditLog[] = [];
  searchResult?: ISourceSearchResult;
  includeDisabled = true;
  includeArchived = false;
  loading = false;
  syncing = false;

  sourceForm: FormGroup = this.fb.group({
    connectorKey: ['email', [Validators.required]],
    name: ['Robert email import', [Validators.required]],
    localOnly: [true],
    syncFrequency: ['manual'],
    excludePatterns: ['spam,trash'],
  });

  importForm: FormGroup = this.fb.group({
    sourceId: ['', [Validators.required]],
    mode: ['manual_import'],
    projectKey: ['018-HAI'],
    externalId: ['sample-1'],
    title: ['Sample connected source item'],
    sourceUri: ['local://sample/source-item'],
    content: ['Decision: use connected sources as structured context. Follow up: verify extracted context before task planning.', [Validators.required]],
  });

  searchForm: FormGroup = this.fb.group({
    query: ['connected sources decisions follow up', [Validators.required]],
    projectKey: ['018-HAI'],
    limit: [8],
    includeSensitive: [false],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(CONNECTED_SOURCE_SERVICE_TOKEN)
    private sourceService: IConnectedSourceService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.loading = true;
    this.sourceService.connectors().subscribe({
      next: (connectors) => (this.connectors = connectors),
      error: () => this.notification.error('Error', 'Failed to load connectors.'),
    });
    this.sourceService.sources(this.includeDisabled).subscribe({
      next: (sources) => {
        this.sources = sources;
        if (!this.importForm.value.sourceId && sources.length) {
          this.importForm.patchValue({ sourceId: sources[0].id });
        }
        this.loading = false;
      },
      error: () => {
        this.loading = false;
        this.notification.error('Error', 'Failed to load sources.');
      },
    });
    this.loadExtractions();
    this.loadAuditLogs();
  }

  connectSource(): void {
    if (this.sourceForm.invalid) {
      return;
    }
    const connector = this.connectors.find(
      (item) => item.connectorKey === this.sourceForm.value.connectorKey
    );
    this.sourceService
      .createSource({
        connectorKey: this.sourceForm.value.connectorKey,
        name: this.sourceForm.value.name,
        category: connector?.category,
        enabled: true,
        localOnly: this.sourceForm.value.localOnly,
        syncFrequency: this.sourceForm.value.syncFrequency,
        excludePatterns: String(this.sourceForm.value.excludePatterns || '')
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean),
      })
      .subscribe({
        next: () => {
          this.notification.success('Source connected', 'The source is ready for controlled sync.');
          this.refresh();
        },
        error: () => this.notification.error('Error', 'Failed to connect source.'),
      });
  }

  sync(): void {
    if (this.importForm.invalid) {
      return;
    }
    this.syncing = true;
    this.sourceService
      .sync(this.importForm.value.sourceId, {
        mode: this.importForm.value.mode,
        items: [
          {
            externalId: this.importForm.value.externalId,
            title: this.importForm.value.title,
            content: this.importForm.value.content,
            sourceUri: this.importForm.value.sourceUri,
            projectKey: this.importForm.value.projectKey,
          },
        ],
      })
      .subscribe({
        next: () => {
          this.syncing = false;
          this.notification.success('Synced', 'Item extracted with provenance.');
          this.refresh();
        },
        error: () => {
          this.syncing = false;
          this.notification.error('Error', 'Sync failed.');
        },
      });
  }

  search(): void {
    if (this.searchForm.invalid) {
      return;
    }
    this.sourceService.search(this.searchForm.value).subscribe({
      next: (result) => (this.searchResult = result),
      error: () => this.notification.error('Error', 'Search failed.'),
    });
  }

  pause(source: IConnectedSource): void {
    this.sourceService.pause(source.id).subscribe(() => this.refresh());
  }

  resume(source: IConnectedSource): void {
    this.sourceService.resume(source.id).subscribe(() => this.refresh());
  }

  reindex(source: IConnectedSource): void {
    this.sourceService.reindex(source.id).subscribe(() => this.refresh());
  }

  revoke(source: IConnectedSource): void {
    if (!window.confirm('Revoke this source access?')) {
      return;
    }
    this.sourceService.revoke(source.id).subscribe(() => this.refresh());
  }

  archive(extraction: ISourceExtraction): void {
    this.sourceService.archiveExtraction(extraction.id).subscribe(() => this.loadExtractions());
  }

  delete(extraction: ISourceExtraction): void {
    if (!window.confirm('Delete this extracted record?')) {
      return;
    }
    this.sourceService.deleteExtraction(extraction.id).subscribe(() => this.loadExtractions());
  }

  markCorrected(extraction: ISourceExtraction): void {
    this.sourceService
      .updateExtraction(extraction.id, {
        ...extraction,
        uncertain: false,
      })
      .subscribe(() => this.loadExtractions());
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }

  private loadExtractions(): void {
    this.sourceService
      .extractions(this.searchForm.value.projectKey, this.includeArchived)
      .subscribe({
        next: (items) => (this.extractions = items),
        error: () => (this.extractions = []),
      });
  }

  private loadAuditLogs(): void {
    this.sourceService.auditLogs().subscribe({
      next: (logs) => (this.auditLogs = logs),
      error: () => (this.auditLogs = []),
    });
  }
}
