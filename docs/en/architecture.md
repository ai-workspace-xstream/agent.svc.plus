# Architecture

This repository mixes a Go runtime agent with supporting deployment automation and edge integration artifacts.

Use this page as the canonical bilingual overview of system boundaries, major components, and repo ownership.

## Current code-aligned notes

- Documentation target: `agent.svc.plus`
- Repo kind: `hybrid-agent`
- Manifest and build evidence: go.mod (`agent.svc.plus`)
- Primary implementation and ops directories: `cmd/`, `internal/`, `agent/`, `deploy/`, `scripts/`, `example/`, `config/`
- Package scripts snapshot: `deploy`

## Existing docs to reconcile

- No directly matching legacy docs were detected; this page is currently the canonical seed.

## What this page should cover next

- Describe the current implementation rather than an aspirational future-only design.
- Keep terminology aligned with the repository root README, manifests, and actual directories.
- Link deeper runbooks, specs, or subsystem notes from the legacy docs listed above.
- Keep diagrams and ownership notes synchronized with actual directories, services, and integration dependencies.
