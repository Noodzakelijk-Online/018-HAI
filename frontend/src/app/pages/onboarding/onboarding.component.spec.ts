import { OnboardingComponent } from './onboarding.component';

describe('OnboardingComponent', () => {
  const router = { navigate: jasmine.createSpy('navigate') } as any;

  beforeEach(() => {
    router.navigate.calls.reset();
    localStorage.removeItem('hai_onboarded');
  });

  it('starts at the first step and clamps at both ends', () => {
    const c = new OnboardingComponent(router);
    expect(c.current).toBe(0);
    c.prev();
    expect(c.current).toBe(0);
    c.next();
    expect(c.current).toBe(1);
  });

  it('recognizes the last step', () => {
    const c = new OnboardingComponent(router);
    c.current = c.steps.length - 1;
    expect(c.isLastStep).toBeTrue();
    c.next();
    expect(c.current).toBe(c.steps.length - 1);
  });

  it('finish marks onboarded and navigates home', () => {
    const c = new OnboardingComponent(router);
    c.finish();
    expect(localStorage.getItem('hai_onboarded')).toBe('true');
    expect(router.navigate).toHaveBeenCalledWith(['/control-center']);
    expect(OnboardingComponent.isOnboarded()).toBeTrue();
  });

  it('uses the shared theme token for its explanatory text', () => {
    const styles = (OnboardingComponent as any).ɵcmp.styles.join('\n');
    expect(styles).toContain('color: var(--hai-muted)');
  });
});
