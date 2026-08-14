# Agent Reconciliation Acceptance

This document defines the execution and acceptance checklist for `agent.svc.plus`
in the CRT-007 control plane.

## P0 Boundary Freeze

- `agent.svc.plus` is scheduler / reconciler / future autoscaling executor only.
- `agent.svc.plus` does not own usage or billing truth.
- `agent.svc.plus` does not read Prometheus as a billing source.

## P1 Scheduling Model

Deliverables:

- periodic trigger for `billing-service` collect-and-rate job
- periodic trigger for `billing-service` reconcile job
- retry and warning logs on downstream failure

Acceptance:

- billing collection job can be triggered by ticker
- reconciliation job can be triggered by ticker
- downstream HTTP failure does not crash the agent loop

## P2 Observability and Status

Deliverables:

- report last job execution time
- report last job success time
- report last job failure message
- keep orchestration status separate from billing truth

Acceptance:

- status report surfaces recent orchestration activity
- restart does not fabricate billing state

## P3 Interface Orchestration

Deliverables:

- invoke `POST /v1/jobs/collect-and-rate`
- invoke `POST /v1/jobs/reconcile`
- never write `traffic_stat_checkpoints`, `traffic_minute_buckets`, `billing_ledger`, or `account_quota_states` directly

Acceptance:

- all billing writes remain inside `billing-service`
- agent only coordinates job execution and reports health

## P4 Runtime Validation

Checks:

- ticker triggers job endpoint on schedule
- retry/backoff path logs warnings and continues
- restart after missed interval resumes scheduling safely
- controller outage or billing-service outage leaves agent degraded but running

Suggested verification commands:

- `cd /Users/shenlan/workspaces/cloud-neutral-toolkit/agent.svc.plus && go test ./...`
- `cd /Users/shenlan/workspaces/cloud-neutral-toolkit/github-org-cloud-neutral-toolkit && rg -n "scheduler|reconciliation|billing-service|does not own billing truth" docs/operations-governance/cross-repo-tasks.md /Users/shenlan/workspaces/cloud-neutral-toolkit/agent.svc.plus/docs`
