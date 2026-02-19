package tools

import (
	"encoding/json"
	"strings"
)

// GenerateFixPlanInput is the input for the generate_fix_plan tool.
type GenerateFixPlanInput struct {
	RegressionType string   `json:"regression_type" jsonschema_description:"Type of regression from detect_regression"`
	Severity       string   `json:"severity" jsonschema_description:"Severity level: critical, high, medium, or low"`
	AffectedFiles  []string `json:"affected_files" jsonschema_description:"Files involved in the regression"`
	RootCause      string   `json:"root_cause" jsonschema_description:"Description of the suspected root cause"`
	Priority       string   `json:"priority" jsonschema_description:"Priority from triage: P0, P1, P2, or P3"`
}

// FixStep represents a single actionable step in the fix plan.
type FixStep struct {
	Order       int    `json:"order"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Automated   bool   `json:"automated"`
}

// GenerateFixPlanOutput contains the actionable fix plan.
type GenerateFixPlanOutput struct {
	ImmediateActions       []string  `json:"immediate_actions"`
	FixSteps               []FixStep `json:"fix_steps"`
	PreventionMeasures     []string  `json:"prevention_measures"`
	ShiftLeftRecommendations []string `json:"shift_left_recommendations"`
	EstimatedEffort        string    `json:"estimated_effort"`
	RollbackPlan           string    `json:"rollback_plan"`
	TestStrategy           string    `json:"test_strategy"`
}

// fixPlaybooks maps regression types to structured fix strategies.
// Each entry reflects the "Get Clean, Stay Clean" principle from Fix Fast.
var fixPlaybooks = map[string]GenerateFixPlanOutput{
	"null_pointer": {
		ImmediateActions: []string{
			"Add null guard before the failing dereference",
			"Check if recent commit removed a non-null guarantee",
		},
		FixSteps: []FixStep{
			{1, "reproduce", "Write a failing test that reproduces the NPE", false},
			{2, "guard", "Add nil/null check with a meaningful error message or default", false},
			{3, "root-cause", "Trace back to where null was first introduced (often in a factory or constructor)", false},
			{4, "annotation", "Add @NullSafe / null-safety annotations to prevent regression", false},
			{5, "test", "Add unit test for the null-input path", true},
		},
		PreventionMeasures: []string{
			"Enable null-safety linter rules (e.g., go vet, staticcheck SA5011)",
			"Require @NullSafe annotation on all new public APIs",
			"Add IDE plugin to flag potential nil dereferences",
		},
		ShiftLeftRecommendations: []string{
			"Enable nil-check warnings as IDE errors (shift detection to IDE stage)",
			"Run staticcheck in pre-commit hook (shift to local_test)",
			"Add nil-pointer detection to CI pipeline",
		},
		EstimatedEffort: "1-4 hours",
		RollbackPlan:    "Revert the commit that removed the non-null guarantee",
		TestStrategy:    "Unit test with nil/zero-value inputs, integration test for the affected flow",
	},
	"performance": {
		ImmediateActions: []string{
			"Check if a recent DB query lost an index",
			"Look for N+1 query patterns introduced in the change",
			"Compare flame graph before and after the change",
		},
		FixSteps: []FixStep{
			{1, "profile", "Run profiler (pprof, perf, py-spy) to get baseline", true},
			{2, "bisect", "Use git bisect to find the commit that caused the regression", true},
			{3, "analyze", "Identify the hot path — DB query, loop, serialization, or allocation", false},
			{4, "optimize", "Apply targeted fix: add index, batch queries, cache result, or reduce allocations", false},
			{5, "benchmark", "Add benchmark test to lock in the performance improvement", true},
			{6, "monitor", "Add performance metric/alert for this path", true},
		},
		PreventionMeasures: []string{
			"Add benchmark tests for all critical code paths",
			"Set performance budgets in CI (e.g., benchstat comparison)",
			"Add DB query explain-plan checks in code review",
		},
		ShiftLeftRecommendations: []string{
			"Run benchmarks in CI and fail on >10% regression (shift to ci stage)",
			"Use continuous profiling in staging before prod promotion",
			"Add performance linting to catch O(n²) patterns statically",
		},
		EstimatedEffort: "4-16 hours (profiling + fix + benchmarks)",
		RollbackPlan:    "Revert to previous release; apply hotfix forward",
		TestStrategy:    "Benchmark tests, load testing with realistic data volumes",
	},
	"crash": {
		ImmediateActions: []string{
			"Enable feature flag to roll back or disable the new code path immediately",
			"Capture full stack trace and correlated logs",
			"Check error monitoring (Sentry, Datadog) for crash volume",
		},
		FixSteps: []FixStep{
			{1, "rollback", "Roll back or disable the change to restore stability", false},
			{2, "reproduce", "Reproduce crash in a local environment using the stack trace", false},
			{3, "fix", "Address root cause — unhandled error, resource exhaustion, or invariant violation", false},
			{4, "test", "Write test that covers the crash scenario", false},
			{5, "canary", "Re-deploy fix to canary/staging before full rollout", true},
		},
		PreventionMeasures: []string{
			"Add crash-rate SLO alert (page when crash rate > baseline)",
			"Enable structured crash reporting with full context",
			"Use feature flags for all risky changes to allow instant rollback",
		},
		ShiftLeftRecommendations: []string{
			"Run chaos/fault-injection tests in CI",
			"Add panic/exception tracking to staging environment",
			"Enable race detector in test runs (go test -race)",
		},
		EstimatedEffort: "2-8 hours for hotfix; 1-3 days for root cause fix",
		RollbackPlan:    "Immediately revert via feature flag; if no flag, revert deployment",
		TestStrategy:    "Crash reproduction test, fault injection, end-to-end scenario test",
	},
	"security_flaw": {
		ImmediateActions: []string{
			"Immediately disable or isolate the affected endpoint/feature",
			"Notify security team (treat as incident)",
			"Assess whether the vulnerability has been exploited (check audit logs)",
		},
		FixSteps: []FixStep{
			{1, "contain", "Disable affected feature or apply WAF rule immediately", false},
			{2, "assess", "Determine blast radius: what data/systems are exposed", false},
			{3, "fix", "Apply security patch with input validation / output encoding / authz check", false},
			{4, "audit", "Full security audit of adjacent code paths", false},
			{5, "pen-test", "Targeted penetration test of the fix", false},
			{6, "disclose", "Coordinate responsible disclosure if external impact", false},
		},
		PreventionMeasures: []string{
			"Add SAST scanner (Semgrep, CodeQL) to CI pipeline",
			"Add DAST scanner against staging before every release",
			"Enforce security code review for all auth/data-handling changes",
		},
		ShiftLeftRecommendations: []string{
			"Run SAST in IDE via plugin (shift to ide stage)",
			"Require security review checklist in PR template",
			"Add automated SQL injection / XSS checks to CI",
		},
		EstimatedEffort: "1-2 days for patch; 1 week for full audit",
		RollbackPlan:    "Immediately revert; do NOT wait for a clean fix if actively exploited",
		TestStrategy:    "OWASP test suite, targeted exploit PoC test, regression test suite",
	},
	"memory_leak": {
		ImmediateActions: []string{
			"Check if recent change added a long-lived reference or disabled GC pressure relief",
			"Monitor heap growth rate in production",
		},
		FixSteps: []FixStep{
			{1, "profile", "Capture heap profile before and after change (pprof heap, valgrind)", true},
			{2, "identify", "Identify the leaking allocation type and call site", false},
			{3, "fix", "Close resources, remove stale references, add defer/finally cleanup", false},
			{4, "test", "Add test that runs the path N times and checks heap growth", false},
			{5, "monitor", "Add heap metric alert for abnormal growth rate", true},
		},
		PreventionMeasures: []string{
			"Add heap memory leak detection to CI (go test with -memprofile)",
			"Require resource cleanup review for all I/O or connection changes",
		},
		ShiftLeftRecommendations: []string{
			"Run go test -memprofile in CI and fail on unexpected heap growth",
			"Enable leak detector in integration tests",
		},
		EstimatedEffort: "4-12 hours",
		RollbackPlan:    "Revert the change; restart affected services to clear leaked memory",
		TestStrategy:    "Memory benchmark, long-running soak test, heap profiling",
	},
}

// GenerateFixPlan produces an actionable fix plan based on regression type and context.
func GenerateFixPlan(inputJSON string) (string, error) {
	var input GenerateFixPlanInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", err
	}

	playbook, ok := fixPlaybooks[input.RegressionType]
	if !ok {
		// Generic fallback playbook.
		playbook = GenerateFixPlanOutput{
			ImmediateActions: []string{
				"Reproduce the issue in a local or staging environment",
				"Identify the most recent change to the affected area",
			},
			FixSteps: []FixStep{
				{1, "reproduce", "Write a failing test case that captures the bug", false},
				{2, "bisect", "Use git bisect to find the introducing commit", true},
				{3, "fix", "Apply targeted fix addressing the root cause", false},
				{4, "test", "Verify the fix with the reproduction test", false},
				{5, "review", "Submit for code review with clear description of root cause and fix", false},
			},
			PreventionMeasures: []string{
				"Add regression test to the test suite",
				"Document the failure mode in the codebase",
			},
			ShiftLeftRecommendations: []string{
				"Add test coverage to catch this class of bug earlier in the pipeline",
				"Consider adding a linter rule to detect this pattern",
			},
			EstimatedEffort:  "Unknown — depends on root cause",
			RollbackPlan:     "Revert the introducing commit",
			TestStrategy:     "Unit test covering the failure scenario",
		}
	}

	// Customize based on severity/priority.
	if input.Priority == "P0" || input.Severity == "critical" {
		playbook.ImmediateActions = append([]string{
			"PAGE ON-CALL IMMEDIATELY — this is a P0 incident",
			"Open incident bridge / war room",
		}, playbook.ImmediateActions...)
	}

	// Annotate affected files into the fix steps.
	if len(input.AffectedFiles) > 0 {
		fileList := strings.Join(input.AffectedFiles, ", ")
		playbook.FixSteps = append(playbook.FixSteps, FixStep{
			Order:       len(playbook.FixSteps) + 1,
			Action:      "focus-files",
			Description: "Focus review on: " + fileList,
			Automated:   false,
		})
	}

	result, err := json.Marshal(playbook)
	return string(result), err
}
