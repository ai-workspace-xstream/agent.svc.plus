# Deployment

This repository mixes a Go runtime agent with supporting deployment automation and edge integration artifacts.

Use this page to standardize deployment prerequisites, supported topologies, operational checks, and rollback notes.

## Current code-aligned notes

- Documentation target: `agent.svc.plus`
- Repo kind: `hybrid-agent`
- Manifest and build evidence: go.mod (`agent.svc.plus`)
- Primary implementation and ops directories: `cmd/`, `internal/`, `agent/`, `deploy/`, `scripts/`, `example/`, `config/`
- Package scripts snapshot: `deploy`

## Existing docs to reconcile

- `mcp-ssh-manager-setup.md`

## What this page should cover next

- Describe the current implementation rather than an aspirational future-only design.
- Keep terminology aligned with the repository root README, manifests, and actual directories.
- Link deeper runbooks, specs, or subsystem notes from the legacy docs listed above.
- Verify deployment steps against current scripts, manifests, CI/CD flow, and environment contracts before each release.
