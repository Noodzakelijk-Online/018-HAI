# External Engine Archive Review

## Decision

The two supplied engine archives were reviewed as **external reference
material**, not imported as a second HAI product. The canonical implementation
remains this repository's Go backend, Angular frontend, and Postgres data
model.

No archive source, dependency lockfile, database schema, credential material,
or generated artifact was copied into this repository.

## Accepted Improvement

The review found that one supplied ZIP has duplicate file entries. That makes
the file set ambiguous: different consumers can resolve the same path
differently, especially on a case-insensitive Windows filesystem.

HAI now rejects OpenClaw ecosystem archives with:

- duplicate normalized paths, including case-only aliases;
- more than 100,000 entries;
- more than 1 GiB of declared uncompressed content; or
- an individual compression ratio above 200:1.

This applies before HAI accepts an owner-uploaded OpenClaw ecosystem path. It
does not extract or execute uploaded content.

## Rejected Archive Behavior

The review deliberately did not port these patterns:

- a parallel React/Express/tRPC/MySQL application, which would recreate the
  second-product problem HAI has already resolved;
- dynamic playbook conditions evaluated as code;
- simulated messaging, calendar, task, desktop-agent, or model outcomes when a
  real provider is absent;
- a pairing-code service that stores short codes directly and lacks HAI's
  existing upstream-authoritative runtime/pairing boundary;
- model routing based on estimated token splits or unmeasured quality scores.

HAI already has the safer replacements: persisted workflows and checklists,
approval gates, read-only source connectors, provider probes, real routing
telemetry, verification status, audit records, and OpenClaw's own pairing and
scope controls.

## Future Import Rule

Current upstream feature accounting and license/security decisions are tracked
in [`external-runtime-feature-parity.md`](external-runtime-feature-parity.md)
and the authenticated Runtime Lab feature-parity API. That inventory does not
weaken the archive rule or authorize code import or execution.

A future external component may be considered only when it has an identifiable
upstream revision and license, a bounded dependency and credential review,
tests against HAI's real interfaces, no simulated-success fallback, and a
clear owner/approval/audit boundary. It must extend the canonical HAI stack;
it must not introduce another independently authoritative dashboard, memory
store, workflow engine, or runtime executor.
