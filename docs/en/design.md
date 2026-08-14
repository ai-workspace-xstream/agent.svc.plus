# Design

This repository mixes a Go runtime agent with supporting deployment automation and edge integration artifacts.

Use this page to consolidate design decisions, ADR-style tradeoffs, and roadmap-sensitive implementation notes.

## Current code-aligned notes

- Documentation target: `agent.svc.plus`
- Repo kind: `hybrid-agent`
- Manifest and build evidence: go.mod (`agent.svc.plus`)
- Primary implementation and ops directories: `cmd/`, `internal/`, `agent/`, `deploy/`, `scripts/`, `example/`, `config/`
- Package scripts snapshot: `deploy`

## Existing docs to reconcile

- `design.md`

## What this page should cover next

- Describe the current implementation rather than an aspirational future-only design.
- Keep terminology aligned with the repository root README, manifests, and actual directories.
- Link deeper runbooks, specs, or subsystem notes from the legacy docs listed above.
- Promote one-off implementation notes into reusable design records when behavior, APIs, or deployment contracts change.
