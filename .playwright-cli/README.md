Local Playwright QA Evidence
============================

This directory contains local browser evidence captured while testing the HAI
login and dashboard shell during the frontend redesign work.

Files
-----

- `console-2026-06-28T16-30-58-116Z.log`: console output from a login-page
  capture, including expected unauthenticated auth checks and a password
  autocomplete warning.
- `console-2026-06-28T16-32-58-803Z.log`: console output from a later capture
  with the same pre-login auth checks.
- `page-2026-06-28T16-32-23-358Z.yml`: accessibility-style page snapshot of the
  HAI shell showing the grouped navigation and top-level dashboard controls.
- `page-2026-06-28T16-31-04-423Z.yml`: failed empty snapshot attempt, retained
  with a note.
- `page-2026-06-28T16-32-59-383Z.yml`: failed empty snapshot attempt, retained
  with a note.

Use
---

Treat this directory as evidence for local QA and regression debugging. It does
not contain application runtime state, credentials, cookies, or source code.
