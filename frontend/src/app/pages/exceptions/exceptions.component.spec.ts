import { ExceptionsComponent } from './exceptions.component';

describe('ExceptionsComponent', () => {
  it('flags only attention states', () => {
    expect(ExceptionsComponent.needsAttention('failed')).toBeTrue();
    expect(ExceptionsComponent.needsAttention('AWAITING_APPROVAL')).toBeTrue();
    expect(ExceptionsComponent.needsAttention('done')).toBeFalse();
    expect(ExceptionsComponent.needsAttention('')).toBeFalse();
  });

  it('filters to exceptions only', () => {
    const c = new ExceptionsComponent();
    c.setItems([
      { id: '1', title: 'ok', state: 'done' },
      { id: '2', title: 'needs approval', state: 'awaiting_approval' },
      { id: '3', title: 'broke', state: 'failed' },
    ]);
    expect(c.exceptions.length).toBe(2);
    expect(c.exceptions.map((e) => e.id)).not.toContain('1');
  });

  it('maps states to tag colors', () => {
    const c = new ExceptionsComponent();
    expect(c.tagColor('failed')).toBe('red');
    expect(c.tagColor('blocked')).toBe('orange');
    expect(c.tagColor('awaiting_approval')).toBe('gold');
    expect(c.tagColor('done')).toBe('default');
  });
});
