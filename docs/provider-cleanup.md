# Real-Provider Cleanup & Account Safety

How to keep real external provider accounts safe, and how to clean up test
artifacts so nothing real is touched by accident.

## Default posture (safe by construction)

- **Paid providers disabled:** `daily_paid_budget_eur: 0`; paid routing is
  impossible until an explicit server-side approval exists (policy-tested).
- **Real account connectors unconfigured by default:** Gmail, Drive, Contacts,
  Calendar, and Trello expose read-only adapters but remain unavailable until
  least-privilege credentials are deliberately supplied and accepted in a
  bounded sandbox run. GitHub supports bounded anonymous/public reads or an
  optional least-privilege token.
- **Provider selection:** `internal/providerfallback` prefers free/local and
  never selects a paid provider unless paid usage is explicitly allowed.

## Cleanup procedures

| Artifact | Cleanup |
| --- | --- |
| Test/demo data | Runs in demo/test mode (`RUN_MODE`), clearly labelled and side-effect free; delete via the normal deletion path. |
| Fake providers | `internal/fakeprovider` is test-only and never wired into production routing. |
| Probe history | Provider probes are read-only and never mutate provider state. |
| Leaked secrets in logs | Redaction strips secrets before logging/persisting; rotate any exposed key (`internal/secretrotation`). |

## Account-safety rules

1. Never store real provider credentials in the repo or `.env` committed to VCS.
2. Rotate provider keys on a schedule (`secretrotation` policy) and immediately on
   suspected exposure.
3. Before enabling any real connector, diff the requested OAuth scopes and record
   the approval.
4. Keep an audit trail (`internal/auditevent`) of any action taken against a real
   provider.

## Verification

`backend doctor` warns when security-sensitive keys are unset; `/system/info`
shows the current run mode so an operator can confirm production vs demo before
enabling real providers.
