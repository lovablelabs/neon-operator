# Go E2E Lifecycle Validation Plan For Neon Operator

## Purpose

This document defines an execution-ready plan to extend the existing Go e2e harness so it validates Neon lifecycle behavior end to end.

The objective is to evolve current infrastructure (`test/e2e`, Ginkgo, Kind, `make test-e2e`, GitHub Actions) and add high-signal lifecycle checks. The objective is not to create a new test framework.

## Problem Statement

Current e2e coverage proves controller deployment and metrics exposure, but does not yet prove full resource lifecycle and runtime usability:

1. `Cluster` reconciles to `Ready=True`.
1. `Project` reconciles to `Ready=True`.
1. `Branch` reconciles to `Ready=True`.
1. Branch compute runtime accepts SQL and returns `SELECT 1 => 1`.

This leaves an important quality gap between operator health and operator correctness.

## Current Baseline In This Repository

### Existing Infrastructure To Reuse

1. `make test-e2e` provisions Kind, runs e2e, and tears Kind down.
1. `test/e2e/e2e_suite_test.go` builds operator/controlplane images and loads both into Kind.
1. `test/e2e/e2e_test.go` already owns deploy/undeploy and test-level diagnostics.
1. `.github/workflows/test-e2e.yml` already executes `make test-e2e` in CI.

### API And Status Contracts That Must Drive Tests

1. API group/version is `neon.oltp.molnett.org/v1alpha1`.
1. Lifecycle resources are `Cluster`, `Project`, and `Branch`.
1. `ClusterSpec` requires referenced secrets for `bucketCredentialsSecret` and `storageControllerDatabaseSecret`.
1. Readiness semantics are condition-driven with `Type=Ready`, consistent with current status helpers.

## Guiding Principles

1. Extend current e2e flow; do not replace it.
1. Validate behavior through public CRD contracts and runtime effects.
1. Keep tests deterministic, serial, and diagnosable before optimizing runtime.
1. Make cleanup part of the assertion surface, not a best-effort afterthought.
1. Grow scenarios incrementally: one stable happy-path first, then targeted variants.

## Target Outcome

The e2e suite must prove the following in one lifecycle scenario:

1. `Cluster`, `Project`, and `Branch` all become `Ready=True`.
1. A compute pod for the created branch is reachable and returns `1` for `SELECT 1`.
1. Failures emit enough logs/events/status snapshots to diagnose in CI without rerun.
1. Teardown deletes all test-created resources and verifies they are gone.

## Scope

### In Scope

1. Add lifecycle-focused spec(s) under `test/e2e`.
1. Add modular e2e helpers for fixtures, waiters, pod exec, diagnostics, and cleanup assertions.
1. Improve `AfterEach` failure collection with Neon-specific detail.
1. Keep existing developer and CI entrypoints (`make test-e2e`) unchanged.

### Out Of Scope For Initial Iteration

1. Replacing Ginkgo/Gomega.
1. Parallelizing lifecycle scenarios.
1. Reworking Kind management or CI topology.
1. Broad performance optimization before stability baseline is established.

## Proposed Test Architecture

### File Layout

1. Keep suite orchestration in `test/e2e/e2e_suite_test.go`.
1. Add lifecycle scenario(s) in `test/e2e/e2e_test.go` under a dedicated context (for example `Context("Neon Lifecycle")`).
1. Add lifecycle helper functions in `test/e2e/helpers_test.go`.
1. Keep generic command utilities in `test/utils/utils.go` and avoid moving lifecycle-specific logic there.

### Lifecycle Happy-Path Flow

1. Precreate required secrets in the test namespace with valid key/value shape.
1. Create `Cluster` CR via typed Go object.
1. Wait until `Cluster` reports `Ready=True`.
1. Create `Project` CR referencing the cluster.
1. Wait until `Project` reports `Ready=True`.
1. Create `Branch` CR referencing the project.
1. Wait until `Branch` reports `Ready=True`.
1. Locate the corresponding compute pod.
1. Execute `psql` in the pod and run `SELECT 1` with retry/backoff.
1. Assert normalized command output is exactly `1`.
1. Teardown resources explicitly in reverse dependency order.
1. Assert lists/gets show no remaining test-created resources.

## Helper Design Contract

Helpers should remain composable and narrow in responsibility.

### Fixture Helpers

1. `buildClusterFixture(...)` returns a valid `v1alpha1.Cluster` with mandatory secret references set.
1. `buildProjectFixture(...)` returns a valid `v1alpha1.Project` that references the created cluster.
1. `buildBranchFixture(...)` returns a valid `v1alpha1.Branch` that references the created project.
1. `buildRequiredSecrets(...)` returns all namespace secrets needed for cluster reconciliation.

### Waiter Helpers

1. `waitForClusterReady(...)`, `waitForProjectReady(...)`, and `waitForBranchReady(...)` should poll with `Eventually` and typed `client.Get`.
1. Readiness checks should assert `Ready=True` and include `Phase` and full conditions snapshot in failure messages.
1. `waitForComputePodReady(...)` should assert `PodReady=True` (and optionally `phase=Running`) for the selected compute pod.

### SQL Exec Helpers

1. `findComputePod(...)` resolves pod name from deterministic labels or naming conventions present in current controller output.
1. `execInPod(...)` executes command in pod (client-go exec preferred; wrapped `kubectl exec` fallback acceptable for first iteration).
1. `assertSelectOne(...)` runs:

```bash
psql -qtAX -h localhost -p 55433 -U cloud_admin -d postgres -c "SELECT 1;"
```

1. SQL assertion retries with bounded timeout and deterministic polling interval.
1. Output normalization trims whitespace/newlines before strict equality check.

### Diagnostics Helpers

1. `dumpLifecycleDiagnostics(...)` is called from `AfterEach` on failure.
1. It should include controller-manager logs, storage-controller logs, compute pod logs, `kubectl get/describe` for lifecycle CRs, and namespace events.
1. Diagnostic collection must be best-effort and never mask the original assertion failure.

### Cleanup Helpers

1. `cleanupLifecycleResources(...)` deletes branch, project, cluster, and created secrets in dependency-safe order.
1. Cleanup should clear pageserver-related finalizers (including pageserver pods) before delete assertions, so teardown cannot stall on terminating resources.
1. `assertLifecycleResourcesDeleted(...)` uses `Eventually` to assert all created objects are absent.
1. Cleanup assertions must print remaining object names, phases, and readiness conditions to simplify triage.

## Phased Delivery Plan

## Phase 1: Introduce Lifecycle Spec Skeleton

### Tasks

1. Add `Neon Lifecycle` context and one happy-path `It(...)` block.
1. Add deterministic naming convention for all test-created resources (for example `cluster-e2e`, `project-e2e`, `branch-e2e`, optional unique suffix).
1. Add typed fixture builders for secrets and lifecycle CRs.

### Acceptance Criteria

1. Test compiles and runs in Kind through `make test-e2e`.
1. Lifecycle resources are created via Go types (`api/v1alpha1`), not inline YAML.
1. Required secret references cannot be accidentally omitted by test authors.

## Phase 2: Add Readiness Waiters

### Tasks

1. Implement typed waiters for `Cluster`, `Project`, and `Branch` readiness.
1. Implement compute pod readiness waiter.
1. Standardize timeout/poll defaults for all lifecycle waiters.

### Acceptance Criteria

1. No hardcoded sleeps are used as primary synchronization.
1. Timeout failures contain object identity, phase, and current condition set.

## Phase 3: Add SQL Connectivity Assertion

### Tasks

1. Implement compute pod locator.
1. Implement pod command execution helper and output normalization.
1. Add bounded retry loop for `SELECT 1` check.

### Acceptance Criteria

1. Test fails if SQL output is empty or not exactly `1`.
1. Startup jitter is tolerated by retries within explicit max duration.

## Phase 4: Harden Diagnostics And Cleanup

### Tasks

1. Expand `AfterEach` diagnostics to include lifecycle CRs and component logs.
1. Add explicit lifecycle resource cleanup function.
1. Add explicit deletion verification checks for all created resources.

### Acceptance Criteria

1. Failed CI logs are actionable for first-pass debugging.
1. Re-runs are not blocked by leftover namespaced test artifacts.
1. Cleanup failures identify exactly which resources remained.

## Phase 5: Stabilize CI Reliability

### Tasks

1. Run repeated `make test-e2e` cycles and tune waiter/exec timeouts.
1. Keep lifecycle scenario serial until flake rate is acceptable.
1. Confirm workflow runtime impact remains acceptable for PR feedback loops.

### Acceptance Criteria

1. Stable pass behavior across repeated CI runs.
1. Runtime remains practical for standard PR workflows.

## Suggested Default Time Budget

Use these as starting defaults and tune after empirical runs:

1. `Eventually` polling interval: `1s`.
1. CR readiness timeout (`Cluster`/`Project`/`Branch`): `8m`.
1. Compute pod readiness timeout: `5m`.
1. SQL connectivity timeout: `3m` with retry interval `5s`.
1. Cleanup verification timeout per resource kind: `2m`.

## CI Diagnostics Minimum Payload

On lifecycle failure, log collection should include at minimum:

1. `kubectl logs` for controller manager pod.
1. `kubectl logs` for storage-controller related pods in test namespace.
1. `kubectl logs` for compute pod selected for SQL assertion.
1. `kubectl get` and `kubectl describe` for created `Cluster`, `Project`, and `Branch`.
1. `kubectl get events --sort-by=.lastTimestamp` for the namespace.
1. A final resource snapshot (`kubectl get all,secret,cluster,project,branch -n <ns>`) scoped to test names where possible.

## Risk Register

1. Startup races between control-plane services and compute availability may cause transient failures.
1. Secret data shape drift can break reconciliation without obvious surface-level errors.
1. Compute pod selection may become brittle if labels/naming conventions drift.
1. CI node/runtime variance can expose latent timing assumptions.

## Mitigations

1. Use readiness + SQL retry helpers with explicit bounded timeouts.
1. Centralize secret fixture creation and validate expected keys before create.
1. Keep compute pod selection logic in one helper with strong failure output.
1. Include condition and phase snapshots in every waiter timeout path.

## Backlog Sequence

1. Add lifecycle context and happy-path skeleton.
1. Add fixture builders and secret fixtures.
1. Add readiness waiters.
1. Add compute pod locator and SQL exec assertion.
1. Add diagnostics aggregator for lifecycle failures.
1. Add cleanup + deletion verification helpers.
1. Tune durations via repeated local `make test-e2e` runs.
1. Validate CI stability and adjust defaults.

## Definition Of Done

1. One lifecycle happy-path test exists in the current Go e2e suite.
1. It validates `Cluster` -> `Project` -> `Branch` readiness and live SQL (`SELECT 1`).
1. It runs via current entrypoint (`make test-e2e`) locally and in CI.
1. Cleanup is explicit and verified; no test-created resources remain.
1. Failure logs include enough context for actionable CI triage.

## Post-Baseline Enhancements

1. Convert happy-path into table-driven scenarios once baseline is stable.
1. Add expected-failure cases (missing secrets, reconciliation error surfaces).
1. Add idempotency and update-propagation lifecycle checks.
1. Consider optional sharding only after flake rate and diagnostics quality are satisfactory.
