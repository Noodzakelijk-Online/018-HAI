import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  ICommandDashboard,
  IMemoryEngineSearchResult,
} from '../../models/memory-engine.model.interface';
import { IMemoryEngineService } from '../../services/memory-engine.service.interface';
import { MEMORY_ENGINE_SERVICE_TOKEN } from '../../services/memory-engine/memory-engine.service.token';

@Component({
  selector: 'app-command-dashboard',
  templateUrl: './command-dashboard.component.html',
  styleUrls: ['./command-dashboard.component.scss'],
})
export class CommandDashboardComponent implements OnInit {
  dashboard?: ICommandDashboard;
  searchResult?: IMemoryEngineSearchResult;
  loading = false;
  searching = false;

  searchForm: FormGroup = this.fb.group({
    query: ['What did we decide about the HAI memory engine?', [Validators.required]],
    projectKey: [''],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(MEMORY_ENGINE_SERVICE_TOKEN) private memoryEngine: IMemoryEngineService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.loading = true;
    this.memoryEngine.dashboard().subscribe({
      next: (dashboard) => {
        this.dashboard = dashboard;
        this.loading = false;
      },
      error: (error) => {
        this.loading = false;
        this.notification.error(
          'Memory engine unavailable',
          error?.error?.error || 'Failed to load the command dashboard.'
        );
      },
    });
  }

  search(): void {
    if (this.searchForm.invalid) {
      return;
    }
    this.searching = true;
    this.memoryEngine
      .search(this.searchForm.value.query, this.searchForm.value.projectKey)
      .subscribe({
        next: (result) => {
          this.searchResult = result;
          this.searching = false;
        },
        error: (error) => {
          this.searching = false;
          this.notification.error('Search failed', error?.error?.error || 'Memory search failed.');
        },
      });
  }

  openSource(sourceUri?: string): void {
    if (sourceUri) {
      window.open(sourceUri, '_blank', 'noopener');
    }
  }

  deleteArchive(id: string, title: string): void {
    if (!window.confirm(`Delete the encrypted archive and extracted facts for "${title}"?`)) {
      return;
    }
    this.memoryEngine.deleteConversation(id).subscribe({
      next: () => {
        this.notification.success('Archive deleted', 'The raw archive and extracted facts were removed.');
        this.refresh();
      },
      error: () => this.notification.error('Delete failed', 'The archive could not be deleted.'),
    });
  }

  openWorkflow(): void {
    this.router.navigate(['/workflow-engine']);
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }
}
