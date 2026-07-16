import { ExceptionsComponent } from './exceptions.component';
import { of } from 'rxjs';

function make(): ExceptionsComponent {
  return new ExceptionsComponent(
    { items: () => of([]) } as any,
    { navigate: jasmine.createSpy('navigate') } as any
  );
}

describe('ExceptionsComponent', () => {
  it('flags only attention states', () => {
    expect(ExceptionsComponent.needsAttention('failed')).toBeTrue();
    expect(ExceptionsComponent.needsAttention('AWAITING_APPROVAL')).toBeTrue();
    expect(ExceptionsComponent.needsAttention('done')).toBeFalse();
    expect(ExceptionsComponent.needsAttention('')).toBeFalse();
  });

  it('filters to exceptions only', () => {
    const c = make();
    c.items = [
      { id: '1', title: 'ok', currentState: 'done' },
      { id: '2', title: 'needs approval', currentState: 'awaiting_approval' },
      { id: '3', title: 'broke', currentState: 'failed' },
    ] as any;
    expect(c.exceptions.length).toBe(2);
    expect(c.exceptions.map((e) => e.id)).not.toContain('1');
  });

  it('maps states to tag colors', () => {
    const c = make();
    expect(c.tagColor('failed')).toBe('red');
    expect(c.tagColor('blocked')).toBe('orange');
    expect(c.tagColor('awaiting_approval')).toBe('gold');
    expect(c.tagColor('done')).toBe('default');
  });

  it('loads workflow records instead of assuming the queue is clear', () => {
    const items = [{ id: 'blocked-1', title: 'Need a reply', currentState: 'blocked' }];
    const c = new ExceptionsComponent(
      { items: jasmine.createSpy().and.returnValue(of(items)) } as any,
      { navigate: jasmine.createSpy('navigate') } as any
    );

    c.refresh();

    expect(c.items).toEqual(items as any);
    expect(c.exceptions.map((item) => item.id)).toEqual(['blocked-1']);
    expect(c.loading).toBeFalse();
  });
});
