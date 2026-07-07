# Accessibility Review

Reviews the dashboard's accessibility and records the concrete a11y measures
applied to the components added in this work.

## Applied in new components (onboarding, exceptions, quick-capture)

- **Landmarks & headings:** each page uses a `<main role="main">` with an
  `aria-labelledby` pointing at a single `<h1>`; sub-sections use real headings.
- **Live regions:** step content and autosave/submit status use `aria-live="polite"`
  so screen readers announce updates.
- **Labelled controls:** every form input has an associated `<nz-form-label
  nzFor>`/`id` pair; error messages are wired via `nzErrorTip`.
- **Tables:** the exceptions table uses `scope="col"` headers and an
  `aria-label` describing its contents.
- **Keyboard:** all actions are native `<button type="button|submit">` elements
  — focusable and operable by keyboard, not click-only `div`s.
- **Progress semantics:** the onboarding steps expose an `aria-label` on the
  step tracker.

## Checklist (WCAG 2.1 AA orientation)

| Area | Status |
| --- | --- |
| Text alternatives / labels | Met in new components |
| Keyboard operability | Met (native buttons/inputs) |
| Focus order & visible focus | Inherited from ng-zorro defaults; verify contrast |
| Color contrast | Uses ng-zorro tokens; **audit needed** for custom greys |
| Status messages announced | Met (aria-live) |
| Headings & landmarks | Met in new components |

## Gaps / follow-ups

1. Run an automated audit (axe-core / Lighthouse) against the running app for
   contrast and ARIA completeness across **existing** pages.
2. Add a visible skip-to-content link in the app shell.
3. Verify focus management on route changes and modal open/close.

## Verdict

New components are built accessible-by-default (landmarks, labels, live regions,
keyboard). A full automated audit across the existing pages is the recommended
next step and is tracked in the roadmap.
