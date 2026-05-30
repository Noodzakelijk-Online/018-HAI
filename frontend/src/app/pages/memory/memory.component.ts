import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  IContextMemory,
  IMemoryRetrieveResult,
} from '../../models/context-memory.model.interface';
import { CONTEXT_MEMORY_SERVICE_TOKEN } from '../../services/context-memory/context-memory.service.token';
import { IContextMemoryService } from '../../services/context-memory.service.interface';

@Component({
  selector: 'app-memory',
  templateUrl: './memory.component.html',
  styleUrls: ['./memory.component.scss'],
})
export class MemoryComponent implements OnInit {
  memories: IContextMemory[] = [];
  retrieveResult?: IMemoryRetrieveResult;
  loading = false;
  saving = false;
  retrieving = false;
  includeArchived = false;
  editingId?: string;

  memoryForm: FormGroup = this.fb.group({
    projectKey: ['018-HAI'],
    kind: ['project', [Validators.required]],
    content: ['', [Validators.required]],
    summary: [''],
    tags: [''],
    confidence: [0.7, [Validators.required, Validators.min(0.1), Validators.max(1)]],
    sourceUri: [''],
    sourceLabel: [''],
  });

  retrieveForm: FormGroup = this.fb.group({
    query: ['LLM routing project preferences', [Validators.required]],
    projectKey: ['018-HAI'],
    limit: [8, [Validators.min(1), Validators.max(20)]],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(CONTEXT_MEMORY_SERVICE_TOKEN)
    private memoryService: IContextMemoryService,
    private notification: NzNotificationService,
    private router: Router
  ) {}

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.loading = true;
    this.memoryService
      .list(this.memoryForm.value.projectKey, this.includeArchived)
      .subscribe({
        next: (memories) => {
          this.memories = memories;
          this.loading = false;
        },
        error: () => {
          this.loading = false;
          this.notification.error('Error', 'Failed to load memories.');
        },
      });
  }

  save(): void {
    if (this.memoryForm.invalid) {
      Object.values(this.memoryForm.controls).forEach((control) => {
        control.markAsDirty();
        control.updateValueAndValidity();
      });
      return;
    }

    this.saving = true;
    const request = this.formRequest();
    const save$ = this.editingId
      ? this.memoryService.update(this.editingId, request)
      : this.memoryService.create(request);

    save$.subscribe({
      next: () => {
        this.saving = false;
        this.clearForm();
        this.refresh();
        this.notification.success('Memory saved', 'Context memory was stored locally.');
      },
      error: () => {
        this.saving = false;
        this.notification.error('Error', 'Failed to save memory.');
      },
    });
  }

  edit(memory: IContextMemory): void {
    this.editingId = memory.id;
    this.memoryForm.patchValue({
      projectKey: memory.projectKey || '',
      kind: memory.kind,
      content: memory.content,
      summary: memory.summary || '',
      tags: memory.tags || '',
      confidence: memory.confidence || 0.7,
      sourceUri: memory.sourceUri || '',
      sourceLabel: memory.sourceLabel || '',
    });
  }

  clearForm(): void {
    this.editingId = undefined;
    this.memoryForm.reset({
      projectKey: '018-HAI',
      kind: 'project',
      content: '',
      summary: '',
      tags: '',
      confidence: 0.7,
      sourceUri: '',
      sourceLabel: '',
    });
  }

  retrieve(): void {
    if (this.retrieveForm.invalid) {
      return;
    }
    this.retrieving = true;
    this.memoryService.retrieve(this.retrieveForm.value).subscribe({
      next: (result) => {
        this.retrieveResult = result;
        this.retrieving = false;
        this.refresh();
      },
      error: () => {
        this.retrieving = false;
        this.notification.error('Error', 'Failed to retrieve memory context.');
      },
    });
  }

  archive(memory: IContextMemory): void {
    if (!memory.id) {
      return;
    }
    this.memoryService.archive(memory.id).subscribe(() => this.refresh());
  }

  restore(memory: IContextMemory): void {
    if (!memory.id) {
      return;
    }
    this.memoryService.restore(memory.id).subscribe(() => this.refresh());
  }

  delete(memory: IContextMemory): void {
    if (!memory.id || !window.confirm('Delete this memory permanently?')) {
      return;
    }
    this.memoryService.delete(memory.id).subscribe(() => this.refresh());
  }

  exportMemories(): void {
    this.memoryService.exportMemories(this.memoryForm.value.projectKey).subscribe({
      next: (data) => {
        const blob = new Blob([JSON.stringify(data, null, 2)], {
          type: 'application/json',
        });
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = '018-hai-context-memory.json';
        link.click();
        window.URL.revokeObjectURL(url);
      },
      error: () => this.notification.error('Error', 'Failed to export memories.'),
    });
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }

  private formRequest() {
    return {
      ...this.memoryForm.value,
      tags: String(this.memoryForm.value.tags || '')
        .split(',')
        .map((tag) => tag.trim())
        .filter(Boolean),
    };
  }
}
