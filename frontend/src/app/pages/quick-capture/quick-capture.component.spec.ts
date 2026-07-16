import { FormBuilder } from '@angular/forms';
import { of, throwError } from 'rxjs';
import { ContextMemoryService } from '../../services/context-memory/context-memory.service';
import { QuickCaptureComponent } from './quick-capture.component';

describe('QuickCaptureComponent', () => {
  beforeEach(() => localStorage.removeItem('hai_quick_capture_draft'));

  function make(memoryService?: Pick<ContextMemoryService, 'create'>): QuickCaptureComponent {
    return new QuickCaptureComponent(
      new FormBuilder(),
      (memoryService || { create: () => of({}) }) as ContextMemoryService
    );
  }

  it('is invalid empty and valid when filled', () => {
    const c = make();
    expect(c.form.valid).toBeFalse();
    c.form.setValue({ title: 'A title', content: 'enough content' });
    expect(c.form.valid).toBeTrue();
  });

  it('enforces content minimum length', () => {
    const c = make();
    c.form.setValue({ title: 'ok', content: 'ab' });
    expect(c.contentControl?.hasError('minlength')).toBeTrue();
  });

  it('autosaves a draft and clears it', () => {
    const c = make();
    c.autosave({ title: 't', content: 'hello there' });
    expect(localStorage.getItem('hai_quick_capture_draft')).toContain('hello there');
    expect(c.savedAt).not.toBeNull();

    c.clearDraft();
    expect(localStorage.getItem('hai_quick_capture_draft')).toBeNull();
    expect(c.savedAt).toBeNull();
  });

  it('does not submit when invalid', () => {
    const c = make();
    c.submit();
    expect(c.submitted).toBeFalse();
  });

  it('stores a valid capture in local memory before clearing the draft', () => {
    const create = jasmine.createSpy().and.returnValue(of({}));
    const c = make({ create });
    c.form.setValue({ title: 'Call solicitor', content: 'Prepare the evidence list.' });

    c.submit();

    expect(create).toHaveBeenCalledWith(jasmine.objectContaining({
      kind: 'note',
      summary: 'Call solicitor',
      content: 'Prepare the evidence list.',
      sourceLabel: 'Quick capture',
    }));
    expect(c.submitted).toBeTrue();
    expect(c.form.value).toEqual({ title: '', content: '' });
  });

  it('retains the form when local memory rejects the capture', () => {
    const c = make({ create: () => throwError(() => new Error('offline')) });
    c.form.setValue({ title: 'Call solicitor', content: 'Prepare the evidence list.' });

    c.submit();

    expect(c.submitted).toBeFalse();
    expect(c.saveError).toContain('Could not save');
    expect(c.form.value.content).toBe('Prepare the evidence list.');
  });
});
