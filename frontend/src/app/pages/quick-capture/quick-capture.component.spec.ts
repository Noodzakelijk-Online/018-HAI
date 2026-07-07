import { FormBuilder } from '@angular/forms';
import { QuickCaptureComponent } from './quick-capture.component';

describe('QuickCaptureComponent', () => {
  beforeEach(() => localStorage.removeItem('hai_quick_capture_draft'));

  function make(): QuickCaptureComponent {
    return new QuickCaptureComponent(new FormBuilder());
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
});
