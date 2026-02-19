---
name: analyze-laas-failures
description: >
  Analyze failed LaaS E2E test CronJobs on a Kubernetes Fragment cluster.
  Fetches pod logs, parses Ginkgo test failures, correlates with recent GitHub
  PRs and releases from all relevant ionos-cloud repositories, and produces a
  root-cause report with BrowserLab-style Wilson confidence scores per suite.
  Invoke with the target namespace.
argument-hint: "<namespace> [suite-name] [--since <days>]"
allowed-tools: Bash(kubectl *), Bash(gh *)
---

# LaaS E2E Failure Analysis

## Arguments

Parse the following from `$ARGUMENTS`:

- **namespace** (required, first positional) — Kubernetes namespace to query
- **suite** (optional, second positional) — filter to one CronJob, e.g. `logging`, `monitoring`, `central-logging`; if omitted analyse all
- **--since N** (optional flag) — look back N days (default: 3, matching `failedJobsHistoryLimit`)

If namespace is missing, ask the user for it before proceeding.

---

## Step 1 — Fetch CronJob and Job State

Run these commands against the cluster. The user is responsible for having the right kubeconfig/context set.

```bash
# List all e2e CronJobs (always labelled test-suite=laas-e2e)
kubectl get cronjobs -n <namespace> -l test-suite=laas-e2e \
  -o json

# List all Jobs spawned by those CronJobs
kubectl get jobs -n <namespace> \
  --sort-by=.metadata.creationTimestamp \
  -o json
```

From the Job list:
- Determine which Jobs are **failed** (`.status.conditions[].type == "Failed"` with `.status == "True"`)
- Determine which Jobs are **completed** (`.status.conditions[].type == "Complete"` with `.status == "True"`)
- Map each Job back to its parent CronJob via `.metadata.ownerReferences[].name`
- Apply the **--since N** filter: discard jobs whose `startTime` is older than N days
- Apply the **suite** filter if provided: only include jobs whose owner CronJob name contains the suite keyword

### 1a — Build Run History per CronJob

For every CronJob (within the time window), reconstruct the run history as an ordered
list of `"pass"` / `"fail"` strings, **oldest first**, from the retained jobs.

Example: a CronJob with jobs `[Complete, Failed, Failed]` oldest→newest → `["pass", "fail", "fail"]`

This run history is the input to the Wilson confidence calculation in Step 5.

---

## Step 2 — Fetch Pod Logs for Failed Jobs

For each **failed** Job (subject to filters), get the pod logs:

```bash
# Get logs from the job's pod (may be terminated/completed)
kubectl logs job/<job-name> -n <namespace> --tail=2000 2>&1

# If the above returns nothing (pod already cleaned up), try:
kubectl get pods -n <namespace> -l job-name=<job-name> -o name
kubectl logs <pod-name> -n <namespace> --tail=2000 2>&1
```

If logs are unavailable (pod GC'd), note that and continue with what is available.

---

## Step 3 — Parse Ginkgo Test Output

The test binaries use **Ginkgo v2**. Learn to recognise these patterns in the logs:

### Failure block
```
[FAILED] in Spec
  <spec-path>

  <failure message>

  In [It] at: <file>:<line> @ <timestamp>
  ...
  Full Stack Trace
    <goroutine stack>
```

### Top-level summary line
```
Ran X of Y Specs in Z seconds
FAIL! -- P Passed | F Failed | 0 Pending | 0 Skipped
```

### Spec path format (nested Describe → Context → It)
```
[LoggingPipelineE2ETest] [Create pipeline] [verifies active state]
```

### What to extract for every failure:
| Field | How to find it |
|---|---|
| Suite name | The Ginkgo suite name printed at startup: `Running Suite: <name>` |
| Spec path | The full `[Outer] [Inner] [It]` path |
| Failure message | The assertion text between `[FAILED]` and `Full Stack Trace` |
| File + line | `In [It] at: path/to/file.go:NN` |
| Timestamp | `@ <RFC3339>` after the file reference |
| Total counts | `P Passed | F Failed` summary line |

### Tip — common failure patterns and what they mean

| Pattern in failure message | Likely area |
|---|---|
| `Expected <string> "" to equal "active"` / `"ready"` / `"healthy"` | Resource not reaching expected state — operator or API issue |
| `Unexpected status code 40x` / `401` / `403` | Auth, RBAC, or token issue |
| `Unexpected status code 50x` | Service crash or misconfiguration |
| `timed out waiting` / `Eventually ... failed` | Slow reconciliation or flake |
| `connection refused` / `no such host` | Networking or DNS issue |
| `EOF` / `i/o timeout` | Infrastructure connectivity |
| `kafka` / `cloudevent` / `event gateway` | Kafka TLS cert or endpoint issue |
| `postgres` / `billing_events` | PostgreSQL connectivity or schema change |
| `grafana` / `dashboard` | Grafana credentials or SA token expired |
| `central.*collision` / `central.*state` | Central services simulator state machine issue |
| `s3` / `minio` / `proxy` | AES proxy or S3 bucket issue |

---

## Step 4 — Fetch Recent Changes from Relevant Repositories

Check **all** of the following ionos-cloud repositories for merged PRs and releases within the **--since N** window. These are the direct dependencies of the test suite plus the test suite itself.

### Primary repos (most likely to cause test failures)

```bash
# 1. The test suite itself — changes here may fix or break tests intentionally
gh pr list --repo ionos-cloud/laas-e2e-cronjob --state merged \
  --limit 30 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/laas-e2e-cronjob --limit 5 --json tagName,createdAt,name

# 2. The logging/monitoring REST API — changes here directly affect test outcomes
gh pr list --repo ionos-cloud/laas-rest-api --state merged \
  --limit 30 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/laas-rest-api --limit 5 --json tagName,createdAt,name

# 3. The K8s operators (fragment + root) — operator regressions cause resource state failures
gh pr list --repo ionos-cloud/laas-operators --state merged \
  --limit 30 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/laas-operators --limit 5 --json tagName,createdAt,name
```

### Secondary repos (shared libraries — lower blast radius but worth checking)

```bash
# 4. Shared LaaS library
gh pr list --repo ionos-cloud/laas-go-pkg --state merged \
  --limit 15 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/laas-go-pkg --limit 5 --json tagName,createdAt,name

# 5. PaaS shared utilities
gh pr list --repo ionos-cloud/go-paaskit --state merged \
  --limit 15 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/go-paaskit --limit 5 --json tagName,createdAt,name

# 6. Event gateway (Kafka) — relevant for event-validation and kafka-related failures
gh pr list --repo ionos-cloud/event-gateway --state merged \
  --limit 15 --json number,title,mergedAt,author,files \
  --search "merged:>$(date -u -v -${SINCE}d +%Y-%m-%d 2>/dev/null || date -u -d "${SINCE} days ago" +%Y-%m-%d)"

gh release list --repo ionos-cloud/event-gateway --limit 3 --json tagName,createdAt,name
```

**Note:** If a repo is private and `gh` returns 404/auth error, note it and continue — do not abort the analysis.

For each PR that has a `files` list, pay attention to:
- Which service/package it touches (`pkg/logging`, `pkg/monitoring`, `pkg/central`, etc.)
- Whether it changes API response shapes, status fields, or timing
- Whether it was merged close in time to the test failure

---

## Step 5 — Compute Wilson Confidence Score per CronJob

For every CronJob, compute a **Wilson score lower bound** from the run history built in
Step 1a. This gives a statistically grounded confidence that the failures are a real
regression (not a flake), regardless of how many keyword signals the failure message
contains. This mirrors the BrowserLab approach: confidence comes from observed runs,
not from heuristics alone.

### Formula (z = 1.0, ~68% confidence interval)

```
p   = failures / total_runs
z   = 1.0
lb  = ( p + z²/2n  −  z·√(p(1−p)/n + z²/4n²) )  /  (1 + z²/n)
wilson_confidence = max(lb, 0)   # clamp to [0, 1]
is_likely_flake   = wilson_confidence < 0.40
```

### Reference values

| Run history (oldest→newest) | failures/total | Wilson score | Verdict |
|---|---|---|---|
| `[fail]` | 1/1 | **0.50** | Regression |
| `[fail, fail]` | 2/2 | **0.67** | Regression |
| `[fail, fail, fail]` | 3/3 | **0.75** | Regression |
| `[fail, fail, fail, fail, fail]` | 5/5 | **0.83** | Regression |
| `[fail, pass, pass]` | 1/3 | **0.14** | Likely flake |
| `[fail, fail, pass]` | 2/3 | **0.39** | Likely flake (borderline) |
| `[fail, pass]` | 1/2 | **0.29** | Likely flake |

### Combining Wilson score with evidence-based confidence

The final reported confidence for each root cause hypothesis uses **both signals**:

| Wilson score | Evidence from Step 4 | Final confidence label |
|---|---|---|
| ≥ 0.67 | PR in primary repo touching failing area | 🔴 **CRITICAL** |
| ≥ 0.50 | PR in primary repo within failure window | 🟠 **HIGH** |
| ≥ 0.50 | PR in secondary repo, or no PR but consistent failure | 🟡 **MEDIUM** |
| < 0.40 | Any evidence | 🟢 **LOW — likely flake** |
| any | No code change found, no pattern match | 🟢 **LOW** |

**Cross-suite correlation rules (raise confidence when met):**
- ≥3 different suites fail with the same error type → platform-wide change; raise each by one level
- Single suite fails repeatedly, all others pass → suite-specific; do not raise
- Failures cluster at the same time of day → consider CronJob overlap or scheduled maintenance

---

## Step 6 — Output Report

Present the analysis in this format:

---

### LaaS E2E Failure Analysis — `<namespace>` — `<date range>`

#### Summary

| CronJob | Suite | Status | Run History | Wilson Score | Verdict | Tests | Failed | Last Run |
|---|---|---|---|---|---|---|---|---|
| `e2e-grafana-permitted-test-cj` | GrafanaPermitted | ❌ FAILED | [fail, fail] | **0.67** | Regression | 5 | 2 | 2026-02-19 06:47 UTC |
| `e2e-central-collision-test-cj` | CentralCollision | ❌ FAILED | [pass, fail, fail] | **0.39** | Likely flake | 8 | 1 | 2026-02-19 07:01 UTC |
| `e2e-logging-test-cj` | LoggingPipeline | ✅ PASSED | [pass, pass, pass] | **—** | — | 22 | 0 | 2026-02-19 06:51 UTC |

---

#### Detailed Failure: `<CronJob Name>` — `<Suite Name>`

**Job:** `<job-name>`  **Pod logs available:** yes/no  **Completed at:** `<timestamp>`
**Run history:** `[pass, fail, fail]` → **Wilson score: 0.39** (likely flake) / **0.67** (regression)

**Failed specs:**
1. `[Outer describe] [context] [It description]` — `<one-line summary of failure>`
2. ...

**Failure details:**
```
<trimmed failure message from Ginkgo, max ~20 lines per failure>
```

**Root Cause Hypotheses:**

| # | Hypothesis | Confidence | Wilson | Evidence |
|---|---|---|---|---|
| 1 | `<concise hypothesis>` | 🔴 CRITICAL / 🟠 HIGH / 🟡 MEDIUM / 🟢 LOW | 0.67 | `<PR/release reference + reasoning>` |

**Recent changes in relevant repos:**

| Repo | PR / Release | Title | Merged/Released | Relevance |
|---|---|---|---|---|
| `laas-rest-api` | PR #123 | Update pipeline status response | 2026-02-18 04:30 | Touches `pkg/logging/status.go` — directly affects tested field |

**Recommended actions:**
1. `<Specific, actionable recommendation based on highest-confidence hypothesis>`
2. `<Second recommendation>`

---

#### Cross-Suite Observations

Note any patterns that span multiple suites here (systemic failures, shared error types, timing clusters, Wilson score correlation).

---

#### No-Data / Skipped

List any CronJobs where logs were unavailable (pod GC'd) or where the suite filter excluded them.
For suites with no retained job history, note Wilson score as `—` (insufficient data).

---

## Notes on cluster access

- All `kubectl` commands use whatever `kubeconfig`/context is currently active in the user's shell. If commands fail due to auth, report the error and suggest `kubectl config use-context <fragment-cluster-context>`.
- The tests run in the namespace they are deployed to (commonly `laas-e2e`, `e2e`, or the fragment cluster's default namespace — use the namespace provided in `$ARGUMENTS`).
- CronJobs keep `failedJobsHistoryLimit: 3` and `successfulJobsHistoryLimit: 5`, so recent history is available but old runs are gone. A Wilson score based on 1 run is valid but weaker than one based on 5 runs — always show `failures/total` alongside the score.
- All test CronJobs share the label `test-suite=laas-e2e` for easy filtering.
