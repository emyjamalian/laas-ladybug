package tools

import (
	"encoding/json"
	"fmt"
)

// Priority levels map to P0-P3 incident severity.
type Priority string

const (
	PriorityP0 Priority = "P0" // Drop everything, fix now
	PriorityP1 Priority = "P1" // Fix today
	PriorityP2 Priority = "P2" // Fix this sprint
	PriorityP3 Priority = "P3" // Backlog
)

// TriageIssueInput is the input for the triage_issue tool.
type TriageIssueInput struct {
	RegressionType      string  `json:"regression_type" jsonschema_description:"Type of regression from detect_regression output"`
	Severity            string  `json:"severity" jsonschema_description:"Severity from detect_regression: critical, high, medium, or low"`
	Environment         string  `json:"environment" jsonschema_description:"Where the issue was found: ide, local_test, ci, code_review, staging, or production"`
	AffectedUsersEstimate int   `json:"affected_users_estimate" jsonschema_description:"Estimated number of users affected (0 if unknown)"`
}

// TriageIssueOutput contains the CPD score and routing decision.
// CPD (Cost Per Developer) is Facebook's metric: bugs cost exponentially more
// the further they are from the point of introduction.
type TriageIssueOutput struct {
	CPDScore          float64  `json:"cpd_score"`
	CPDMultiplier     int      `json:"cpd_multiplier"`
	Priority          Priority `json:"priority"`
	RecommendedAction string   `json:"recommended_action"`
	ShiftLeftTarget   string   `json:"shift_left_target"`
	CostRationale     string   `json:"cost_rationale"`
}

// environmentMultiplier reflects the cost escalation model from Fix Fast.
// A bug caught in production is ~100x more expensive than one caught in the IDE.
var environmentMultiplier = map[string]int{
	"ide":          1,
	"local_test":   3,
	"ci":           10,
	"code_review":  15,
	"staging":      30,
	"production":   100,
}

var severityBaseScore = map[string]float64{
	"critical": 100.0,
	"high":     50.0,
	"medium":   20.0,
	"low":      5.0,
}

// TriageIssue calculates the Cost Per Developer score and assigns priority.
func TriageIssue(inputJSON string) (string, error) {
	var input TriageIssueInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", err
	}

	multiplier, ok := environmentMultiplier[input.Environment]
	if !ok {
		multiplier = 30 // default to staging-level cost
	}

	baseScore, ok := severityBaseScore[input.Severity]
	if !ok {
		baseScore = 20.0
	}

	// CPD = base_severity_score × environment_multiplier × user_impact_factor
	userImpactFactor := 1.0
	if input.AffectedUsersEstimate > 1000 {
		userImpactFactor = 2.0
	} else if input.AffectedUsersEstimate > 100 {
		userImpactFactor = 1.5
	}

	cpdScore := baseScore * float64(multiplier) * userImpactFactor

	var priority Priority
	var action string
	switch {
	case cpdScore >= 5000:
		priority = PriorityP0
		action = "Page on-call immediately. Revert or hotfix within 1 hour."
	case cpdScore >= 1000:
		priority = PriorityP1
		action = "Fix today. Assign to the last committer and block release if unresolved."
	case cpdScore >= 200:
		priority = PriorityP2
		action = "Schedule fix this sprint. Add to team backlog with owner assigned."
	default:
		priority = PriorityP3
		action = "Add to backlog. Consider addressing during next refactoring cycle."
	}

	// Recommend the 'shift left' target: what stage would have caught this earlier?
	shiftLeft := shiftLeftTarget(input.Environment)

	output := TriageIssueOutput{
		CPDScore:          cpdScore,
		CPDMultiplier:     multiplier,
		Priority:          priority,
		RecommendedAction: action,
		ShiftLeftTarget:   shiftLeft,
		CostRationale: fmt.Sprintf(
			"Base severity score %.0f × %dx environment multiplier (found in %s) × %.1fx user impact = CPD %.0f. "+
				"If caught at %s stage, CPD would have been %.0f (%.0fx cheaper).",
			baseScore, multiplier, input.Environment, userImpactFactor, cpdScore,
			shiftLeft, baseScore*float64(environmentMultiplier[shiftLeft])*userImpactFactor,
			cpdScore/(baseScore*float64(environmentMultiplier[shiftLeft])*userImpactFactor),
		),
	}

	result, err := json.Marshal(output)
	return string(result), err
}

func shiftLeftTarget(env string) string {
	targets := map[string]string{
		"production":  "staging",
		"staging":     "ci",
		"ci":          "local_test",
		"code_review": "local_test",
		"local_test":  "ide",
		"ide":         "ide", // already as left as possible
	}
	if t, ok := targets[env]; ok {
		return t
	}
	return "ci"
}
