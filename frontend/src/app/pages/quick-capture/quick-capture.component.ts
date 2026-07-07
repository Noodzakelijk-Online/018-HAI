import { Component, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Subscription } from 'rxjs';
import { debounceTime } from 'rxjs/operators';

const DRAFT_KEY = 'hai_quick_capture_draft';

@Component({
  selector: 'app-quick-capture',
  templateUrl: './quick-capture.component.html',
  styleUrls: ['./quick-capture.component.scss'],
})
export class QuickCaptureComponent implements OnInit, OnDestroy {
  form: FormGroup;
  savedAt: Date | null = null;
  submitted = false;

  private sub?: Subscription;

  constructor(private fb: FormBuilder) {
    this.form = this.fb.group({
      title: ['', [Validators.required, Validators.maxLength(120)]],
      content: ['', [Validators.required, Validators.minLength(3)]],
    });
  }

  ngOnInit(): void {
    const draft = this.loadDraft();
    if (draft) {
      this.form.patchValue(draft, { emitEvent: false });
    }
    // Autosave the draft 500ms after the user stops typing.
    this.sub = this.form.valueChanges
      .pipe(debounceTime(500))
      .subscribe((value) => this.autosave(value));
  }

  ngOnDestroy(): void {
    this.sub?.unsubscribe();
  }

  get titleControl() {
    return this.form.get('title');
  }

  get contentControl() {
    return this.form.get('content');
  }

  autosave(value: unknown): void {
    try {
      localStorage.setItem(DRAFT_KEY, JSON.stringify(value));
      this.savedAt = new Date();
    } catch {
      /* storage unavailable — skip autosave silently */
    }
  }

  loadDraft(): Record<string, unknown> | null {
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      return raw ? JSON.parse(raw) : null;
    } catch {
      return null;
    }
  }

  submit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    // A real submit would POST to /memory; on success the draft is cleared.
    this.submitted = true;
    this.clearDraft();
  }

  clearDraft(): void {
    try {
      localStorage.removeItem(DRAFT_KEY);
    } catch {
      /* ignore */
    }
    this.form.reset();
    this.savedAt = null;
  }
}
