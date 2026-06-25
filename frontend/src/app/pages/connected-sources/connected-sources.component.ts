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
  ISourceSyncResult,
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
    connectorKey: ['local-folder', [Validators.required]],
    name: ['Selected local folder', [Validators.required]],
    localOnly: [true],
    syncFrequency: ['manual'],
    syncTarget: ['.'],
    defaultProjectKey: ['018-HAI'],
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

  folderForm: FormGroup = this.fb.group({
    sourceId: ['', [Validators.required]],
    mode: ['incremental_sync'],
    projectKey: ['018-HAI'],
    folderPath: ['.', [Validators.required]],
    limit: [100],
    maxBytes: [1048576],
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
        if (!this.folderForm.value.sourceId && sources.length) {
          const localFolder = sources.find((source) => source.connectorKey === 'local-folder');
          this.folderForm.patchValue({ sourceId: (localFolder || sources[0]).id });
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
        syncTarget: this.sourceForm.value.syncTarget,
        defaultProjectKey: this.sourceForm.value.defaultProjectKey,
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
        error: (error) => this.notification.error('Error', error?.error?.error || 'Failed to connect source.'),
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
        next: (result) => {
          this.syncing = false;
          this.notifySyncResult('Item sync', result);
          this.refresh();
        },
        error: () => {
          this.syncing = false;
          this.notification.error('Error', 'Sync failed.');
        },
      });
  }

  connectorLabel(connector: ISourceConnector): string {
    const status = connector.adapterStatus || (connector.enabled ? 'operational' : 'not_implemented');
    return `${connector.name} (${status})`;
  }

  connectorChanged(connectorKey: string): void {
    if (connectorKey === 'json-feed') {
      this.sourceForm.patchValue({
        name: 'Local account JSON bridge',
        syncFrequency: '15m',
        syncTarget: 'http://host.docker.internal:8787/feed',
        localOnly: true,
      });
      return;
    }
    if (connectorKey === 'local-folder') {
      this.sourceForm.patchValue({
        name: 'Selected local folder',
        syncFrequency: 'manual',
        syncTarget: '.',
        localOnly: true,
      });
    }
  }

  syncTargetPlaceholder(): string {
    return this.sourceForm.value.connectorKey === 'json-feed'
      ? 'Allowlisted HTTP(S) JSON feed URL'
      : 'Folder target, e.g. .';
  }

  syncSource(source: IConnectedSource): void {
    this.syncing = true;
    this.sourceService
      .sync(source.id, {
        mode: 'incremental_sync',
        items: [],
        projectKey: source.defaultProjectKey,
      })
      .subscribe({
        next: (result) => {
          this.syncing = false;
          this.notifySyncResult('Source sync', result);
          this.refresh();
        },
        error: (error) => {
          this.syncing = false;
          this.notification.error('Source sync failed', error?.error?.error || 'The connector could not retrieve records.');
        },
      });
  }

  syncFolder(): void {
    if (this.folderForm.invalid) {
      return;
    }
    this.syncing = true;
    this.sourceService
      .sync(this.folderForm.value.sourceId, {
        mode: this.folderForm.value.mode,
        items: [],
        folderPath: this.folderForm.value.folderPath,
        projectKey: this.folderForm.value.projectKey,
        limit: Number(this.folderForm.value.limit || 100),
        maxBytes: Number(this.folderForm.value.maxBytes || 1048576),
      })
      .subscribe({
        next: (result) => {
          this.syncing = false;
          this.notifySyncResult('Folder sync', result);
          this.refresh();
        },
        error: () => {
          this.syncing = false;
          this.notification.error('Error', 'Folder sync failed.');
        },
      });
  }

  runDueScheduledSyncs(): void {
    this.syncing = true;
    this.sourceService.runDueScheduledSyncs().subscribe({
      next: (result) => {
        this.syncing = false;
        const summary = `${result.completed} completed, ${result.failed} failed, ${result.skipped} skipped.`;
        if (result.failed > 0) {
          this.notification.warning('Scheduled sync requires attention', summary);
        } else {
          this.notification.success('Scheduled sync checked', summary);
        }
        this.refresh();
      },
      error: () => {
        this.syncing = false;
        this.notification.error('Error', 'Scheduled sync check failed.');
      },
    });
  }

  private notifySyncResult(label: string, result: ISourceSyncResult): void {
    const summary = `${result.job.itemsSeen} seen, ${result.job.itemsFailed || 0} failed. ${result.message}`;
    if (result.job.status === 'completed') {
      this.notification.success(label, summary);
      return;
    }
    this.notification.warning(`${label} requires attention`, summary);
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
