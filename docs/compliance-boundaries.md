# Compliance & Platform Policy Boundaries

Defines what 018-HAI will and will not do automatically, so automation stays
within platform terms and legal constraints.

## Operating stance

Local-first and approval-gated. The system prepares work but does not perform
real external actions without an explicit human approval and, where required, a
verified credential.

## Boundaries

| Area | Boundary |
| --- | --- |
| Paid providers | No paid LLM/API usage until server-side approval; default budget €0 (`daily_paid_budget_eur: 0`). |
| Real account connectors (Gmail/Drive/Calendar/Trello/GitHub) | Disabled until OAuth scopes are minimal and reviewed; sandbox adapters first. |
| Automation execution | Bounded, allowlisted, approval-gated, and verified; emergency stop can halt it. |
| Scraping / third-party sites | Only within the target platform's terms; no evasion of access controls. |
| Personal data | Handled per `docs/privacy-impact-assessment.md`; retention per policy. |

## Platform policy alignment

- Respect rate limits and quotas of any external provider (provider quota +
  budget ledger).
- Never present a demo/test action as a real one (`demomode` labelling; no-fake
  rules).
- Keep a truthful, redaction-aware audit trail of actions taken.

## What is explicitly out of scope

- Circumventing authentication, CAPTCHAs, or paywalls.
- Bulk unsolicited outreach.
- Any action a provider's terms prohibit for automation.

When a provider cannot be automated within policy, the system builds an assisted
workflow that prepares everything and tells the user what remains manual — it
never pretends the manual step happened.
